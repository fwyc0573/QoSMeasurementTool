package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/urfave/cli"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"
)

const (
	BUFLEN = 350
)

type Packet struct {
	Data []int8  // 随机数据
	Timestamp int64 // 时间戳
}

type Result struct {
	targetNode string
	sendPacket int64
	recvPacket int64
	averageDelay interface{}
	lossRate float64
	pressBandwidth float64
	jitter int64
}


func main() {
	app := cli.NewApp()
	app.EnableBashCompletion = true
	app.Name = "inspectENV client"
	app.Version = "1.0.1"
	app.Description = "A system that obtain the realtime QoS of nodes and services"
	app.Commands = []cli.Command{
		{
			Name: "node",
			Aliases: []string{"n"},
			Usage: "inspect the nodes status",
			UsageText: "node",
			Action: measureNodes,
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:  "address, a",
					Value: &cli.StringSlice{},
					Usage: "target socket",
				},
				cli.Int64Flag{
					Name: "num, n",
					Value: 100,
					Usage: "packet number",
				},
				cli.IntFlag{
					Name: "deadLine, d",
					Value: 2,
					Usage: "Max waiting time of receive a packet (in second)",
				},
				cli.IntFlag{
					Name: "timeInterval, i",
					Value: 5,
					Usage: "Time interval of sending a packet (in microsecond)",
				},
				cli.IntFlag{
					Name: "packetSize, s",
					Value: BUFLEN,
					Usage: "size of a packet",
				},
			},
		},
	}
	app.RunAndExitOnError()
}


func measureNodes(c *cli.Context) error {

	//if c.NArg() ==0{
	//	return cli.NewExitError("Target node address is not provided!",1)
	//}
	addresses := c.StringSlice("address")
	packetNum := c.Int64("num")
	deadLine := c.Int64("deadLine")
	timeInterval := c.Int64("timeInterval")
	packetSize := c.Int("packetSize")
	wg := sync.WaitGroup{}
	//fmt.Printf("The number of routine: %v\n", cap(addresses)*2)
	wg.Add(cap(addresses))
	results := make([]Result, cap(addresses))

	for i, address := range addresses{
		address := address
		i := i
		go func() {
			result, err := measureNode(address, packetNum, deadLine, timeInterval, packetSize)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			results[i] = result
			wg.Done()
		}()
	}
	wg.Wait()

	fmt.Printf("Index \t   Target \t Average delay \t Loss rate \t Jitter \t Pressure bandwidth\n")
	for index, result := range results {
		fmt.Printf("  %v    %v \t     %v ms \t    %v%% \t\t %v ms \t\t %.2f Mbit/s \n",
			index, result.targetNode, result.averageDelay, result.lossRate, result.jitter, result.pressBandwidth)
	}


	return nil
}


func measureNode(address string, packetNum int64,
	deadLine int64, timeInterval int64, packetSize int) (Result ,error){
	var result Result

	// 将套接字地址化
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return result, err
	}
	// 监听一个udp连接
	conn, err := net.DialUDP("udp",nil, serverAddr)
	if err != nil {
		return result, err
	}
	wgg := sync.WaitGroup{}
	wgg.Add(2)

	//defer conn.Close()

	var totalWrite int64 = 0
	var totalRead int64 = 0
	var totoalWriteTime int64 = 0

	var recv_count int64 = 0
	var recv_missed_count int64 = 0
	var sum_delay int64 = 0
	var max_delay int64 = 0
	var min_delay int64 = 0
	var curr_delay int64 = 0


	go func() {
		data := make([]int8, packetSize)
		rand.Seed(time.Now().Unix())
		for i:=0;i<packetSize;i++ {
			data[i] = int8(rand.Intn(255))
		}

		for cnt:=packetNum;cnt>0;cnt--{
			timestamp := time.Now().UnixNano() // 纳秒
			var buf bytes.Buffer
			encoder := gob.NewEncoder(&buf)
			sendData := &Packet{data,timestamp/1e3}

			err := encoder.Encode(sendData)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			n, err := conn.Write(buf.Bytes())
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			totalWrite += int64(n)
			time.Sleep(time.Duration(timeInterval)*time.Nanosecond)

			endTimestamp := time.Now().UnixNano() // 纳秒
			totoalWriteTime = totoalWriteTime + endTimestamp-timestamp
			//fmt.Printf("Send No.%dth packet, sendData is %v, length is %v\n", packetNum-cnt,sendData.Timestamp,n)
		}
		wgg.Done()
	}()

	//time.Sleep(time.Second)


	go func() {
		for  {
			conn.SetReadDeadline(time.Now().Add(time.Second*time.Duration(deadLine)))
			buf := make([]byte, 1024*1024)
			n,_ ,err := conn.ReadFromUDP(buf)
			// fmt.Println(err)
			if err != nil {
				//fmt.Println(err)
				e, ok := err.(net.Error)
				if !ok || !e.Timeout() {
					// 非超时的错误
					fmt.Println(err)
					os.Exit(1)
				} else if e.Timeout() {
					recv_missed_count++
					//fmt.Println("No packet returned!")
					break
					//if recv_missed_count == 3 {
					//	break
					//}
					//continue
				}
			}
			curr_time := time.Now().UnixNano() / 1e3 // 微秒
			totalRead += int64(n)
			// 处理接收到的包
			decoder := gob.NewDecoder(bytes.NewReader(buf[:n]))
			p := Packet{}
			decoder.Decode(&p)
			curr_delay = int64(curr_time) - int64(p.Timestamp)
			//fmt.Println(buf[:n])
			recv_count++
			sum_delay += curr_delay

			if recv_count == 1 {
				max_delay = curr_delay
				min_delay = curr_delay
			} else if recv_count>1 {
				if curr_delay>max_delay {
					max_delay = curr_delay
				}
				if curr_delay<min_delay {
					min_delay = curr_delay
				}
			}
			//fmt.Printf("No. %d: current timestamp is %v, recieved data is %v, length is %v\n", recv_count,curr_time,p.Timestamp,n)
		}
		wgg.Done()
	}()
	wgg.Wait()

	pressBand := float64(totalWrite)*(1e9/(1024*1024))*8/float64(totoalWriteTime)   //MB/s

	var average_delay interface{}

	if recv_count == 0{
		average_delay = "---"
	} else{
		average_delay = float64((sum_delay/1000)/recv_count)
	}

	result.targetNode = address
	result.recvPacket = recv_count
	result.averageDelay = average_delay
	result.jitter = (max_delay-min_delay)/1000
	result.lossRate = (float64(packetNum-recv_count)/float64(packetNum))*100
	result.sendPacket = packetNum
	result.pressBandwidth = pressBand


	return result, nil
}