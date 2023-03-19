package main

import (
	"fmt"
	"time"
	"runtime"
	"sync"
)

func test(n int64)(int64, int64){

	send := time.After(time.Millisecond*800)
	recv := time.After(time.Millisecond*1000)

	var sendCount int64 = 0
	var recvCount int64 = 0

	go func() {
		for  {
			select {
			case <-recv:
				runtime.Goexit()
			default:
				recvCount++
				fmt.Printf("###Recieve No.%vth packet.\n", n+recvCount)
				time.Sleep(time.Millisecond*100)
			}
		}
	}()

	//time.Sleep(time.Millisecond*800)

	fmt.Printf("#This is %vth routine.\n", n)
	go func() {
		for  {
			select {
			case <- send:
				fmt.Println("End!!!!!!!!!!")
				runtime.Goexit()
			default:
				sendCount++
				fmt.Printf("###Send No.%vth packet.\n", n+sendCount)
				time.Sleep(time.Millisecond*100)
			}
		}
	}()
	//time.Sleep(time.Second*1)

	return sendCount,recvCount

}


func main() {

	var i int64

	wg := sync.WaitGroup{}
	wg.Add(5)



	for i=1;i<6;i++{
		i := i
		go func() {
			sendCount,recvCount:=test(i*1000)
			fmt.Println(sendCount,recvCount)
			wg.Done()
		}()
	}
	wg.Wait()


}