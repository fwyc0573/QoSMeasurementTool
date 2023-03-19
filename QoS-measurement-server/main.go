package main

import (
	"fmt"
	"github.com/urfave/cli"
	"net"
	"os"
	"strconv"
	"time"
)


func main() {
	app := cli.NewApp()
	app.EnableBashCompletion = true
	app.Name = "inspectENV server"
	app.Version = "1.0.1"
	app.Description = "A system that obtain the realtime QoS of nodes and services"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "listen, l",
			Value: "0.0.0.0",
			Usage: "listening ip",
		},
		cli.IntFlag{
			Name: "port, p",
			Value: 6000,
			Usage: "listening port",
		},
		cli.IntFlag{
			Name: "deadLine, d",
			Value: 15,
			Usage: "time interval of send packet",
		},
	}
	app.Commands = []cli.Command{
		{
			Name: "server",
			Aliases: []string{"s"},
			Usage: "inspect the nodes status",
			UsageText: "node",
			Action: listenNode,
		},
	}
	app.RunAndExitOnError()
}

func listenNode(c *cli.Context) error {
	source := c.Parent().String("listen")
	port := c.Parent().Int("port")
	deadLine := c.Parent().Int64("deadLine")

	address := source + ":" + strconv.Itoa(port)
	// 将套接字地址化
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	// 监听一个udp连接
	conn, err := net.ListenUDP("udp",serverAddr)
	if err != nil {
		return err
	}

	defer conn.Close()

	var recv_count int64 = 0

	for  {
		conn.SetReadDeadline(time.Now().Add(time.Second*time.Duration(deadLine)))
		buf := make([]byte, 1024*1024)
		n,src,err := conn.ReadFromUDP(buf)
		//fmt.Println(n)
		if err != nil {

			e, ok := err.(net.Error)
			if !ok || !e.Timeout() {
				// 非超时的错误
				fmt.Println(err)
				os.Exit(1)
			}
			if recv_count !=0 {
				fmt.Printf("Recieve %v packets.\n",recv_count)
				recv_count = 0
			}
			continue
		}
		if n > 0 {
			recv_count++
			_,err := conn.WriteToUDP(buf[:n],src)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			//fmt.Printf("send No.%dth packet back, size is %v \n", recv_count,n)
		}
	}

	return nil
}
