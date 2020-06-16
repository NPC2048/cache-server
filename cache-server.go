package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/bradfitz/gomemcache/memcache"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"
)

// memcached 客户端
var mc *memcache.Client

// 配置文件结构
var Config = struct {
	HashServerHost string   `yaml:"hash-server-host"`
	Port           string   `default:"80"`
	MemcachedHost  []string `yaml:"memcached-host"`
}{}

func init() {
	// 初始化读取配置文件
	initLoadConfig()
	// 初始化 memcached
	initMemcached()
}

func initLoadConfig() {
	configName := "config.yml"
	ext, err := PathExists(configName)
	if err != nil {
		panic(err)
	}
	if !ext {
		fmt.Println("配置文件: ", configName, " 不存在")
		os.Exit(-2)
	}
	yamlData, err := ioutil.ReadFile(configName)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-2)
	}
	err = yaml.Unmarshal(yamlData, &Config)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(-2)
	}
	if len(Config.HashServerHost) < 1 {
		fmt.Println("hash-server-host 不能为空")
		os.Exit(-2)
	}
}

func initMemcached() {
	// 检查 memcached-host 是否为空
	if len(Config.MemcachedHost) < 1 {
		fmt.Println("memcached-host 不能为空")
		os.Exit(-2)
	}
	// 检查 memcached 数组中是否有空的
	for index, host := range Config.MemcachedHost {
		if len(host) < 1 {
			fmt.Println("memcached-host[", index, "] 不能为空")
			os.Exit(-2)
		}
	}
	// 从配置文件读取 memcached 地址
	mc = memcache.New(Config.MemcachedHost...)
}

// hash 缓存
func HashCacheServer(writer http.ResponseWriter, request *http.Request) {
	// 解析请求参数
	input := request.FormValue("input")
	// md5 key
	md5Hash := md5.Sum([]byte(input))
	key := hex.EncodeToString(md5Hash[:])
	// 判断 input 是否已存在
	item, err := mc.Get(key)
	if err != nil {
		fmt.Println(err)
	}
	if item != nil {
		// 直接返回
		_, _ = writer.Write(item.Value)
		return
	}
	//请求 hash-server
	resp, err := Forward(request)
	if err != nil {
		fmt.Println(err)
	} else {
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		_, _ = writer.Write(body)
		// 缓存 input
		_ = mc.Set(&memcache.Item{Key: key, Value: body})
	}
}

// 其他请求，按照请求路径, 请求方法, 请求 header, 请求参数, 请求内容原封不动的转发至目标服务器
func Proxy(writer http.ResponseWriter, request *http.Request) {
	// 请求转发
	targetResponse, _ := Forward(request)
	// 设置 response header 信息
	defer targetResponse.Body.Close()
	for key, val := range targetResponse.Header {
		for _, value := range val {
			writer.Header().Add(key, value)
		}
	}
	data, _ := ioutil.ReadAll(targetResponse.Body)
	_, _ = writer.Write(data)
}

func main() {
	wg := sync.WaitGroup{}
	fmt.Println("Begin start cache-server...")
	_ = StartHttpServer()
	fmt.Println("Run Done.")
	wg.Add(1)
	wg.Wait()
}

func StartHttpServer() *http.Server {
	serve := &http.Server{Addr: ":" + Config.Port}
	// calc 路径的请求进行缓存处理
	http.HandleFunc("/calc", HashCacheServer)
	// 其他路径直接传给 server
	http.HandleFunc("/", Proxy)
	go func() {
		if err := serve.ListenAndServe(); err != nil {
			fmt.Println(err.Error())
			// 关闭服务器
			_ = serve.Shutdown(nil)
			os.Exit(-2)
		}
	}()
	return serve
}

// 转发方法
func Forward(request *http.Request) (*http.Response, error) {
	proxy := func(_ *http.Request) (*url.URL, error) {
		return url.Parse(Config.HashServerHost)
	}
	transport := &http.Transport{
		Proxy: proxy,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConnsPerHost:   100,
	}
	client := &http.Client{Transport: transport}
	target := "http://" + request.RemoteAddr + request.RequestURI
	targetRequest, _ := http.NewRequest(request.Method, target, request.Body)
	//fmt.Println("target:" + target)
	// 设置 header 信息
	for key, val := range request.Header {
		for _, value := range val {
			targetRequest.Header.Add(key, value)
		}
	}
	// 请求并返回数据
	return client.Do(targetRequest)
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
