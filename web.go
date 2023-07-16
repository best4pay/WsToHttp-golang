package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
)

var ws *websocket.Conn       //定义websocket
var sendQueue chan []byte    //定义发送队列
var receiveQueue chan []byte //定义接收队列
var error_int = 0            //记录错误次数

func main() {

	time.Sleep(3000 * time.Millisecond) //3秒后启动

	sendQueue = make(chan []byte, 0xffff)    // 初始化发送队列长度65535
	receiveQueue = make(chan []byte, 0xffff) // 初始化发送队列长度65535

	// 运行web
	go runWeb()

	// 建立 WebSocket 连接并处理连接中断的逻辑
	go runWebSocket()

	// 持续运行，防止程序退出
	select {}

}

func runslef() {
	// 获取可执行文件的路径
	executable, err := os.Executable()
	if err != nil {
		fmt.Println("获取可执行文件路径失败:", err)
		return
	}

	log.Println("executable:", executable)

	// 重新启动程序
	cmd := exec.Command(executable, os.Args[1:]...)
	err = cmd.Start()
	if err != nil {
		fmt.Println("重新启动程序失败:", err)
		return
	}

	// 关闭当前程序
	os.Exit(0)
}

func runWeb() {
	http.HandleFunc("/", handleRequest)
	log.Println(http.ListenAndServe(":8080", nil))
}

func runWebSocket() {
	var err error // 声明 err 变量

	// WebSocket 服务器地址
	url := "ws://127.0.0.1:9000"
	url = "ws://192.168.1.9:8090/ws/self/3c2cc301-93c3-47af-9088-4f1750543d69/"

	// 设置 WebSocket 连接的请求头
	header := http.Header{}
	header.Set("Authorization", "Bearer your-token")

	// 创建一个 Dialer 实例
	dialer := websocket.DefaultDialer

	// 使用 Dialer 建立 WebSocket 连接
	ws, _, err = dialer.Dial(url, header)
	if err != nil {
		log.Println("无法连接到 WebSocket:", err)
		time.Sleep(5 * time.Second) // 等待一段时间后重新连接
		go runWebSocket()
		return
	} else {
		log.Println("WebSocket连接成功")
	}
	defer ws.Close()

	go receive() //接收消息

	var msg []byte
	for { //这个循环会一直等待,直到有消息可以发送
		msg = <-sendQueue
		//向ws服务器发送的消息
		color.Cyan("向ws服务器发送的消息:")
		color.Magenta("%s\n", string(msg))
		err = ws.WriteMessage(websocket.TextMessage, msg) //取出并发送队列
		if err != nil {
			log.Println("发送消息失败:", err)
			go runWebSocket()
			return
		}

	}

}

func send(message []byte) {
	sendQueue <- message //插入队列
}

func receive() {
	for {
		_, receivedMessage, err := ws.ReadMessage()
		if err != nil {
			log.Println("接收消息失败:", err)
			return
		}

		// 打印接收到的消息
		color.Cyan("ws服务器发来的消息:")
		color.Magenta("%s\n", string(receivedMessage))

		//log.Printf("变量类型: %T\n", receivedMessage)

		// 解析接收到的 JSON 数据
		var data map[string]interface{}
		err = json.Unmarshal(receivedMessage, &data)
		if err != nil {
			log.Println("json数据解析失败:", err)
			continue
		}

		log.Println("接收到的ws数据:", string(receivedMessage))

		nonce_hash, ok := data["nonce_hash"].(string)

		log.Println("nonce_hash:", nonce_hash)

		// 判断键是否存在
		callbackURL, ok := data["callback_url"].(string)
		if ok { //如果url存在就post url地址回调

			client_order_no, ok := data["data"].(map[string]interface{})["client_order_no"].(string)
			uuid, ok := data["uuid"].(string)
			json_type, ok := data["type"].(string)
			if !ok {
				//log.Println("client_order_no:", client_order_no)
				continue
			}

			// 删除指定的键值对
			delete(data, "callback_url")

			// 将 map 转换为 JSON 字符串
			jsonStr, err := json.Marshal(data)
			if err != nil {
				log.Println("无法转换map到json:", err)
				continue
			}

			// 发送 POST 请求
			resp, err := http.Post(callbackURL, "application/json;charset=utf-8", bytes.NewBuffer(jsonStr))
			if err != nil {
				log.Println("发送POST请求失败:", err)
				continue
			}
			defer resp.Body.Close()

			color.Cyan("http请求地址:")
			color.Magenta("%s\n", callbackURL)

			// 处理响应
			color.Cyan("http响应状态:")
			color.Magenta("%s\n", resp.Status)

			// 读取响应的内容
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Println("无法读取响应正文:", err)
				continue
			}
			color.Cyan("http响应 Body:")
			color.Magenta("%s\n", string(body))

			jsonData := `{"type": "%s","nonce_hash": "%s","data": {"client_order_no": "%s"},"result": "%s","msg": %s,"uuid":"%s"}` //构建一个json

			json_body, err := json.Marshal(body)
			if err != nil {
				log.Println("JSON 编码失败:", err)
			}

			log.Println("JSON内容:", string(json_body))

			if resp.StatusCode == http.StatusOK { //页面正常返回,通知ws服务器
				send(body) //向服务器发送回调成功
				log.Println("发送的内容:", string(body))
			} else {

				json := fmt.Sprintf(jsonData, json_type, nonce_hash, client_order_no, "fail", json_body, uuid) //替换变量到json中
				send([]byte(json))
				log.Println("发送的内容:", json) //向服务器发送回调失败
			}

		} else {
			// 写入接收队列
			receiveQueue <- receivedMessage
			//log.Println("没找到url:", string(receivedMessage))
		}
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法是否为POST
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("无效的请求方法"))
		return
	}

	// 读取请求中的JSON数据
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("无法读取请求正文"))
		return
	}

	//log.Printf("body变量类型: %T\n", body)

	color.Cyan("http接收到的JSON:")
	color.Magenta("%s\n", body)

	send(body) //ws发送消息

	// 解析接收到的 JSON 数据
	var data1 map[string]interface{}

	err = json.Unmarshal(body, &data1)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("无法读取请求正文" + err.Error()))
		return
	}
	nonce_hash2 := ""
	// 从解析后的 map 中提取值
	nonce_hash1, ok := data1["nonce_hash"].(string)
	if !ok {
		w.Write([]byte("无法获取 nonce_hash"))
		return
	}

	for i := 0; i <= 100; i++ { //10秒超时

		//log.Println("循环次数:", i)

		//取出目标数据,把非目标数据重新插入到队列中
		for i := 0; i < len(receiveQueue); i++ {
			data := <-receiveQueue
			//log.Println("ws接收数据队列", string(data))
			var data2 map[string]interface{}
			err = json.Unmarshal(data, &data2)
			if err != nil {
				log.Println([]byte("json格式不正确无法读取请求正文" + err.Error()))
				continue
			}

			// 从解析后的 map 中提取值
			nonce_hash2, ok = data2["nonce_hash"].(string)
			if !ok {
				//log.Panicln("无法获取 nonce_hash")
				receiveQueue <- data //如果没有nonce_hash那么就插回队列
				continue
			}

			// 判断是否是目标数据
			if nonce_hash2 == nonce_hash1 { //hash相同说明找到目标数据
				// 设置响应头
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200) //返回200
				w.Write(data)      //写入从队列中取出对应nonce_hash的内容返回给页面
				color.Cyan("向http返回的JSON:")
				color.Magenta("%s\n", data)
			} else {
				// 将非目标数据插入到当前队列后面
				receiveQueue <- data

			}
		}

		if nonce_hash2 == nonce_hash1 { //hash相同说明找到目标数据
			log.Println("队列中剩余数量:", len(receiveQueue))
			return
		}

		time.Sleep(100 * time.Millisecond) //0.1秒

	}

	w.WriteHeader(408)
	w.Write([]byte("超时 错误:408")) //这里显示超时页面
}
