package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// IPResult 检测结果结构体
type IPResult struct {
	IP      string        // IP地址
	Alive   bool          // 是否可达
	Latency time.Duration // 连接耗时
}

// 并发控制：最大同时检测20个IP
const maxConcurrency = 20

func main() {
	// 1. 读取IP文件
	ipList, err := readIPFile("ip.txt")
	if err != nil {
		fmt.Printf("\n❌ 错误：读取IP文件失败 -> %v\n", err)
		waitForExit() // 报错后等待用户输入，不退出终端
		return
	}
	if len(ipList) == 0 {
		fmt.Println("\n❌ 错误：ip.txt 文件中未检测到任何有效IP地址")
		waitForExit()
		return
	}

	fmt.Printf("✅ 成功加载 %d 个IP地址，开始检测...\n\n", len(ipList))

	// 2. 并发检测IP
	results := concurrentCheckIP(ipList)

	// 3. 输出所有检测结果
	fmt.Println("========== 全部IP检测结果 ==========")
	for _, res := range results {
		status := "❌ 不可达"
		if res.Alive {
			status = fmt.Sprintf("✅ 可达   耗时: %v", res.Latency)
		}
		fmt.Printf("%-18s %s\n", res.IP, status)
	}

	// 4. 筛选并排序最优IP
	bestIPs := getTopBestIPs(results, 5)
	fmt.Println("\n========== 连接效果最好的前5个IP ==========")
	if len(bestIPs) == 0 {
		fmt.Println("⚠️  未检测到任何可达的IP地址")
	} else {
		for i, ip := range bestIPs {
			fmt.Printf("第%d名: %-18s 耗时: %v\n", i+1, ip.IP, ip.Latency)
		}
	}

	// 程序正常结束，等待用户退出
	waitForExit()
}

// readIPFile 读取ip.txt，返回有效IP列表
func readIPFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ipList []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		if ip != "" {
			ipList = append(ipList, ip)
		}
	}
	return ipList, scanner.Err()
}

// concurrentCheckIP 高并发检测IP连通性
func concurrentCheckIP(ipList []string) []IPResult {
	concurrencyChan := make(chan struct{}, maxConcurrency)
	resultChan := make(chan IPResult, len(ipList))
	var wg sync.WaitGroup

	for _, ip := range ipList {
		concurrencyChan <- struct{}{}
		wg.Add(1)
		go func(ip string) {
			defer func() {
				<-concurrencyChan
				wg.Done()
			}()
			resultChan <- checkIP(ip)
		}(ip)
	}

	// 异步关闭结果通道
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	var results []IPResult
	for res := range resultChan {
		results = append(results, res)
	}
	return results
}

// checkIP 检测单个IP（TCP 80端口，500ms超时）
func checkIP(ip string) IPResult {
	start := time.Now()
	// 拼接IP+端口
	address := net.JoinHostPort(ip, "80")
	conn, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	latency := time.Since(start)

	if err != nil {
		return IPResult{IP: ip, Alive: false, Latency: latency}
	}
	defer conn.Close()
	return IPResult{IP: ip, Alive: true, Latency: latency}
}

// getTopBestIPs 获取耗时最短的前N个可达IP
func getTopBestIPs(results []IPResult, top int) []IPResult {
	var aliveIPs []IPResult
	for _, res := range results {
		if res.Alive {
			aliveIPs = append(aliveIPs, res)
		}
	}

	// 按耗时升序排序
	sort.Slice(aliveIPs, func(i, j int) bool {
		return aliveIPs[i].Latency < aliveIPs[j].Latency
	})

	// 限制返回数量
	if len(aliveIPs) < top {
		top = len(aliveIPs)
	}
	return aliveIPs[:top]
}

// waitForExit 等待用户按下回车键，防止终端直接退出（核心优化）
func waitForExit() {
	fmt.Println("\n========================================")
	fmt.Println("程序执行完毕，按 回车键 退出...")
	// 读取用户输入
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}