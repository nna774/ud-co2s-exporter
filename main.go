package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tarm/serial"
)

const (
	namespace = "udco2s"
)

var (
	co2 = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "CO2",
		Help:      "CO2",
	})
	humid = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "HUM",
		Help:      "humidity",
	})
	temp = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "TEMP",
		Help:      "temperature",
	})
	last = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "last",
		Help:      "last time value get successfully",
	})

	sensorReg = regexp.MustCompile(`CO2=(\d+),HUM=(\d+\.\d+),TMP=(\d+\.\d+)`)
)

func newPort(path string) (*serial.Port, error) {
	c := &serial.Config{
		Name:        path,
		Baud:        115200,
		ReadTimeout: 6 * time.Second,
	}
	return serial.OpenPort(c)
}

func newCollector(path string) (*collecter, error) {
	port, err := newPort(path)
	if err != nil {
		return nil, err
	}
	if _, err := port.Write([]byte("STA\r\n")); err != nil {
		return nil, err
	}
	c := &collecter{port: port, v: &values{}}
	go func() {
		for {
			c.updateValues()
		}
	}()
	return c, nil
}

type values struct {
	co2   float64
	humid float64
	temp  float64
	last  float64
}

type collecter struct {
	port *serial.Port
	v    *values
	s    *bufio.Scanner
}

func (c *collecter) updateValues() {
	if c.s == nil {
		c.s = bufio.NewScanner(c.port)
	}
	c.s.Scan()
	t := c.s.Text()
	fmt.Printf("### scan: %v\n", t)
	res := sensorReg.FindStringSubmatch(t)
	if len(res) < 4 {
		fmt.Printf("got wrong response: %+v\n", res)
		return
	}

	// atomicに更新したほうがいいかもしれないけど、次の値と混ざって問題ある？
	co2, err := strconv.ParseFloat(res[1], 64)
	if err != nil {
		fmt.Printf("co2 parse failed: %v\n", res[0][0])
		return
	}
	c.v.co2 = co2
	hum, err := strconv.ParseFloat(res[2], 64)
	if err != nil {
		fmt.Printf("hum parse failed: %v\n", res[0][1])
		return
	}
	c.v.humid = hum
	temp, err := strconv.ParseFloat(res[3], 64)
	if err != nil {
		fmt.Printf("temp parse failed: %v\n", res[0][2])
		return
	}
	c.v.temp = temp

	c.v.last = float64(time.Now().Unix())
}

func (c *collecter) Describe(ch chan<- *prometheus.Desc) {
	ch <- co2.Desc()
	ch <- humid.Desc()
	ch <- temp.Desc()
	ch <- last.Desc()
}

func (c *collecter) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(
		co2.Desc(),
		prometheus.GaugeValue,
		c.v.co2,
	)
	ch <- prometheus.MustNewConstMetric(
		humid.Desc(),
		prometheus.GaugeValue,
		c.v.humid,
	)
	ch <- prometheus.MustNewConstMetric(
		temp.Desc(),
		prometheus.GaugeValue,
		c.v.temp,
	)
	ch <- prometheus.MustNewConstMetric(
		last.Desc(),
		prometheus.CounterValue,
		c.v.last,
	)
}

func main() {
	if len(os.Args) < 2 {
		panic(fmt.Sprintf("%v UD-CO2S-SERIAL-PATH", os.Args[0]))
	}
	path := os.Args[1]
	c, err := newCollector(path)
	if err != nil {
		panic(err)
	}
	prometheus.MustRegister(c)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":5000", nil))
}
