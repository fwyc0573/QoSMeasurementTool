package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/urfave/cli"
	"math/rand"
	"net"
	"os"
	"runtime"
	"strconv"
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
	jitter interface{}
	sumDelay float64
}


func main() {
	app := cli.NewApp()
	app.EnableBashCompletion = true
	app.Name = "inspectENV client"
	app.Version = "1.0.1"
	app.Description = "A system that obtain the realtime QoS of nodes or services, each test cost <sendTime+deadLine> seconds"
	app.Commands = []cli.Command{
		{
			Name: "node",
			Aliases: []string{"n"},
			Usage: "inspect the nodes status, each test cost <sendTime+deadLine> seconds",
			UsageText: "node",
			Action: measureNodes,
			Flags: []cli.Flag{
				cli.StringSliceFlag{
					Name:  "address, a",
					Value: &cli.StringSlice{},
					Usage: "target socket",
				},
				cli.IntFlag{
					Name: "sendTime,st",
					Value: 2,
					Usage: "Time of send packets for one test.(in second)",
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
	addresses := c.StringSlice("address")
	deadLine := c.Int64("deadLine")
	timeInterval := c.Int64("timeInterval")
	packetSize := c.Int("packetSize")
	sendTime := c.Int64("sendTime")
	if len(addresses) == 0 {
		fmt.Println("Target node address is not provided!")
		os.Exit(1)
	}
	wg := sync.WaitGroup{}
	for  {
		wg.Add(cap(addresses))
		results := make([]Result, cap(addresses))
		for i, address := range addresses{
			address := address
			i := i
			go func() {
				result, err := measureNode(address, time.Duration(deadLine)*time.Second, timeInterval, packetSize, time.Duration(sendTime)*time.Second)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				results[i] = result
				wg.Done()
			}()
		}
		wg.Wait()

		fmt.Printf("Index \t   Target \t AverDlay \t LossRate \t Jitter \t TansRate \t sendCount\n")
		for index, result := range results {
			fmt.Printf("  %v    %v \t %v ms \t   %.2f%% \t %.2f ms \t%.2f Mbit/s \t    %v\n",
				index, result.targetNode, result.averageDelay, result.lossRate, result.jitter, result.pressBandwidth,result.sendPacket)
		}
	}
}

func measureNode(address string, deadLine time.Duration, timeInterval int64,
	packetSize int, sendTime time.Duration) (Result ,error){
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

	//defer conn.Close()

	var totalWrite int64 = 0
	var totalRead int64 = 0
	var totalWriteTime int64 = 0

	var recvCount int64 = 0
	var sendCount int64 = 0
	var recvMissCount int64 = 0
	var sumDelay float64 = 0
	var maxDelay float64 = 0
	var minDelay float64 = 0
	var currDelay float64 = 0

	sendDDL := time.After(sendTime)
	recvDDL := time.After(sendTime+deadLine)

	// Send packets to remote server
	go func() {
		data := make([]int8, packetSize)
		rand.Seed(time.Now().Unix())
		for i:=0;i<packetSize;i++ {
			data[i] = int8(rand.Intn(255))
		}

		for {
			select {
			case <-sendDDL:
				runtime.Goexit()
			default:
				timestamp := time.Now().UnixNano() // 纳秒
				var buf bytes.Buffer
				encoder := gob.NewEncoder(&buf)
				sendData := &Packet{data,timestamp}
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
				sendCount++
				time.Sleep(time.Duration(timeInterval)*time.Nanosecond)

				endTimestamp := time.Now().UnixNano() // 纳秒
				totalWriteTime = totalWriteTime + endTimestamp-timestamp
				//fmt.Printf("Send No.%dth packet, sendData is %v ns, length is %v\n", sendCount,sendData.Timestamp,n)
			}
		}
	}()

	// Receive packet from remote server 8272135161
	go func() {
		for  {
			select {
			case <-recvDDL:
				runtime.Goexit()
			default:
				conn.SetReadDeadline(time.Now().Add(deadLine))
				buf := make([]byte, 1024*1024)
				n,_ ,err := conn.ReadFromUDP(buf)
				if err != nil {
					e, ok := err.(net.Error)
					if !ok || !e.Timeout() {
						// 非超时的错误
						fmt.Println(err)
						os.Exit(1)
					} else if e.Timeout() {
						recvMissCount++
						//fmt.Println("No packet returned!")
						break
						//if recvMissCount == 3 {
						//	break
						//}
						//continue
					}
				}
				currTime := time.Now().UnixNano() // 微秒
				totalRead += int64(n)
				// 处理接收到的包
				decoder := gob.NewDecoder(bytes.NewReader(buf[:n]))
				p := Packet{}
				decoder.Decode(&p)
				currDelay = float64(currTime - p.Timestamp)/1e6 //毫秒
				//fmt.Println(buf[:n])
				recvCount++
				sumDelay += currDelay

				if recvCount == 1 {
					maxDelay = currDelay
					minDelay = currDelay
				} else if recvCount>1 {
					if currDelay>maxDelay {
						maxDelay = currDelay
					}
					if currDelay<minDelay {
						minDelay = currDelay
					}
				}
				//fmt.Printf("No. %d: current timestamp is %v ns, received data is %v ns, currDelay is %v ms, length is %v\n", recvCount,currTime,p.Timestamp,currDelay,n)
			}
		}
	}()

	time.Sleep(sendTime+deadLine)
	pressBand := float64(totalWrite)*(1e9/(1024*1024))*8/float64(totalWriteTime)   //MB/s

	var averDelay interface{}
	var jitter interface{}
	if recvCount == 0{
		averDelay = "---"
		jitter = "---"
	} else{
		averDelay, _ = strconv.ParseFloat(fmt.Sprintf("%.2f",sumDelay/float64(recvCount)), 64)
		jitter, _ = strconv.ParseFloat(fmt.Sprintf("%.2f",maxDelay-minDelay), 64)
	}

	result.targetNode = address
	result.recvPacket = recvCount
	result.averageDelay = averDelay
	result.jitter = jitter
	result.lossRate = (float64(sendCount-recvCount)/float64(sendCount))*100
	result.sendPacket = sendCount
	result.pressBandwidth = pressBand
	result.sumDelay = sumDelay


	return result, nil
}