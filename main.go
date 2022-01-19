package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 定义常量
var (
	DockerClient *client.Client
)

// 定义容器运行状态，方便grafana展示时，不同的值展示不同的颜色
const (

	CREATED = "created"
	RESTARTING = "restarting"
	EXITED = "exited"
	RUNNING = "running"
	UNKNOW = "noknow"

	UNKNOWStateValue = 0.1
	CreatedStateValue = 0.2
	RestartingStateValue = 0.4
	ExitedStateValue = 0.6
	RunningStateValue = 1
)

var ContainerStatusMap = map[string]float64{
	CREATED: CreatedStateValue,
	RESTARTING: RestartingStateValue,
	EXITED: ExitedStateValue,
	RUNNING: RunningStateValue,
	UNKNOW: UNKNOWStateValue,
}

// 1. 定义一个结构体，用于存放描述信息
type Exporter struct {
	queryDockerStatus *prometheus.Desc
}

// 2. 定义一个Collector接口，用于存放两个必备函数，Describe和Collect
type Collector interface {
	Describe(chan<- *prometheus.Desc)
	Collect(chan<- prometheus.Metric)
}

// 3. 定义两个必备函数Describe和Collect
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	// 将描述信息放入队列
	ch <- e.queryDockerStatus
}

func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	for _, info := range GetContainerList() {
		//log.Println(info)
		ch <- prometheus.MustNewConstMetric(
			e.queryDockerStatus,
			prometheus.GaugeValue,
			GetContainerStateValue(info.State),
			strings.TrimPrefix(info.Names[0], "/"), // 指标的标签值与NewDesc中的第三个参数一样对应
			info.ID,
			info.Image,
			info.Status,
			info.State,
			GetContainerVersion(info.Image),
		)
	}
}

// 5. 定义一个实例化函数，用于生成prometheus数据
func NewExporter() *Exporter {
	return &Exporter{
		queryDockerStatus: prometheus.NewDesc(
			"container_run_state",                                //指标名称
			"query container status ",                              // 指标help信息
			[]string{"name", "id", "image", "status", "state","version"}, 		// 指标的label名称
			nil),
	}
}

func GetContainerList() (containerList []types.Container) {

	containerList, err := DockerClient.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		log.Printf("connect docker server err, %#v", err)
		return
	}
	return
}

func GetContainerVersion(image string) (version string) {
	split := strings.Split(image, ":")
	if len(split) > 1 && strings.Contains(image, "aiforward") {
		version = split[1]
	}
	return
}

func GetContainerStateValue(state string) (value float64) {
	if v,ok := ContainerStatusMap[state]; ok {
		value = v
		return
	}
	value = ContainerStatusMap[UNKNOW]
	return
}

var (
	address = flag.String("listen-address", ":9417", "The address to listen on for HTTP requests.")
)

func InitDockerConnect() {
	//c, err := client.NewClientWithOpts(client.WithVersion("1.38"), client.WithHost("tcp://10.100.3.206:2375"))
	c, err := client.NewClientWithOpts(client.WithVersion("1.38"))
	log.Println("init docker server connect")
	defer func() {
		if err := c.Close();err != nil {
			log.Printf("client close err:%v\n", err)
		}
	}()
	if err != nil {
		log.Printf("connect docker server err, %#v\n", err)
		return
	}

	DockerClient = c
}

func init() {
	InitDockerConnect()
}

func main() {
	//GetContainerList()
	// 6. 实例化并注册数据采集器exporter
	workerA := NewExporter()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(workerA)

	// 7. 定义一个采集数据的采集器集合，它可以合并多个不同的采集器数据到一个结果集合中
	gatherers := prometheus.Gatherers{
		// prometheus.DefaultGatherer,  // 默认的数据采集器，包含go运行时的指标信息
		reg,
	}

	// 8. start http server
	h := promhttp.HandlerFor(gatherers,
		promhttp.HandlerOpts{
			ErrorHandling: promhttp.ContinueOnError,
		})
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		log.Println("start http metrics...")
		h.ServeHTTP(w, r)
	})

	//flag.Parse()
	//address = flag.String("listen-address", ":9417", "The address to listen on for HTTP requests.")
	server := &http.Server{Addr: *address, Handler: nil}

	go func() {
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Printf("start server err, error message: %#v", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, os.Interrupt)
	<-quit
	log.Println("Server shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Println("message", fmt.Sprintf("Failed to gracefully shutdown: %v", err))
	}
	log.Println("Server shutdown")
}
