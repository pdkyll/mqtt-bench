package main

import (
	"bytes"
	"flag"
	"fmt"
	MQTT "git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
	"os"
	"sync"
	"time"
)

// 実行オプション
type ExecOptions struct {
	Broker      string // Broker URI
	ClientNum   int    // クライアントの同時実行数
	Count       int    // 1クライアント当たりのメッセージ数
	MessageSize int    // 1メッセージのサイズ(byte)
	Qos         byte   // QoS(0/1/2)
}

func Execute(exec func(clients []*MQTT.Client, opts ExecOptions, param ...string), opts ExecOptions) {
	message := CreateFixedSizeMessage(opts.MessageSize)

	clients := make([]*MQTT.Client, opts.ClientNum)
	hasErr := false
	for i := 0; i < opts.ClientNum; i++ {
		client := Connect(opts.Broker, i)
		if client == nil {
			hasErr = true
			break
		}
		clients[i] = client
	}

	// 接続エラーがあれば、接続済みのクライアントの切断処理を行い、処理を終了する。
	if hasErr {
		for i := 0; i < len(clients); i++ {
			client := clients[i]
			if client != nil {
				Disconnect(client)
			}
		}
		return
	}

	// 安定させるために、一定時間待機する。
	time.Sleep(3 * time.Second)

	startTime := time.Now().Nanosecond()
	exec(clients, opts, message)
	endTime := time.Now().Nanosecond()

	for i := 0; i < len(clients); i++ {
		Disconnect(clients[i])
	}

	// 処理結果を出力する。
	totalCount := opts.ClientNum * opts.Count
	duration := (endTime - startTime) / 1000000                  // nanosecond -> millisecond
	throughput := float64(totalCount) / float64(duration) * 1000 // messages/sec
	fmt.Printf("\nPublish result : broker=%s, clients=%d, count=%d, duration=%dms, throughput=%.2fmessages/sec\n",
		opts.Broker, opts.ClientNum, opts.Count, duration, throughput)
}

// 全クライアントに対して、publishの処理を行う。
func PublishAllClient(clients []*MQTT.Client, opts ExecOptions, param ...string) {
	message := param[0]

	wg := new(sync.WaitGroup)

	for id := 0; id < len(clients); id++ {
		client := clients[id]
		wg.Add(1)

		go func() {
			defer wg.Done()

			for index := 0; index < opts.Count; index++ {
				// fmt.Printf("Publish : id=%d, count=%d\n", id, index)
				Publish(client, "/go-mqtt/benchmark/"+string(id)+"/"+string(index), opts.Qos, message)
			}
		}()
	}

	wg.Wait()
}

// メッセージを送信する。
func Publish(client *MQTT.Client, topic string, qos byte, message string) {
	token := client.Publish(topic, qos, false, message)

	if token.Wait() && token.Error() != nil {
		fmt.Printf("Publish error: %s\n", token.Error())
	}
}

// 全クライアントに対して、subscribeの処理を行う。
func SubscribeAllClient(clients []*MQTT.Client, opts ExecOptions, param ...string) {
	wg := new(sync.WaitGroup)

	for id := 0; id < len(clients); id++ {
		client := clients[id]
		wg.Add(1)

		go func() {
			defer wg.Done()

			for index := 0; index < opts.Count; index++ {
				// fmt.Printf("Subscribe : id=%d, count=%d\n", id, index)
				Subscribe(client, "/go-mqtt/benchmark/"+string(id)+"/"+string(index), opts.Qos)
			}
		}()
	}

	wg.Wait()
}

// メッセージを受信する。
func Subscribe(client *MQTT.Client, topic string, qos byte) {
	token := client.Subscribe(topic, qos, nil)

	if token.Wait() && token.Error() != nil {
		fmt.Printf("Subscribe error: %s\n", token.Error())
	}

}

// 固定サイズのメッセージを生成する。
func CreateFixedSizeMessage(size int) string {
	var buffer bytes.Buffer
	for i := 0; i < size; i++ {
		buffer.WriteString(string(i % 10))
	}

	message := buffer.String()
	return message
}

// 指定されたBrokerへ接続し、そのMQTTクライアントを返す。
// 接続に失敗した場合は nil を返す。
func Connect(broker string, id int) *MQTT.Client {
	opts := MQTT.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID("mqtt-benchmark" + string(id))

	client := MQTT.NewClient(opts)
	token := client.Connect()

	if token.Wait() && token.Error() != nil {
		fmt.Printf("Connected error: %s\n", token.Error())
		return nil
	}

	return client
}

// Brokerとの接続を切断する。
func Disconnect(client *MQTT.Client) {
	client.ForceDisconnect()
}

func main() {
	broker := flag.String("broker", "tcp://{host}:{port}", "URI of MQTT broker (required)")
	action := flag.String("action", "p/pub/publish or s/sub/subscribe", "Publish or Subscribe (required)")
	clients := flag.Int("clients", 10, "Number of clients")
	count := flag.Int("count", 100, "Number of loops")
	size := flag.Int("size", 1024, "Message size per publish (byte)")
	qos := flag.Int("qos", 0, "MQTT QoS(0/1/2)")
	flag.Parse()

	if len(os.Args) <= 1 {
		flag.Usage()
		return
	}

	// validate "broker"
	if broker == nil || *broker == "" || *broker == "tcp://{host}:{port}" {
		fmt.Printf("Invalid argument : -broker -> %s\n", *broker)
		return
	}

	// validate "action"
	var method string = ""
	if *action == "p" || *action == "pub" || *action == "publish" {
		method = "pub"
	} else if *action == "s" || *action == "sub" || *action == "subscribe" {
		method = "sub"
	}

	if method != "pub" && method != "sub" {
		fmt.Printf("Invalid argument : -action -> %s\n", *action)
		return
	}

	execOpts := ExecOptions{}
	execOpts.Broker = *broker
	execOpts.ClientNum = *clients
	execOpts.Count = *count
	execOpts.MessageSize = *size
	execOpts.Qos = byte(*qos)

	switch method {
	case "pub":
		Execute(PublishAllClient, execOpts)
	case "sub":
		Execute(SubscribeAllClient, execOpts)
	}
}
