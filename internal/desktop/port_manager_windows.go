//go:build windows

package desktop

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

// CheckPortOccupied 检查端口是否被占用，返回占用进程的 PID
// 如果端口未被占用，返回 -1
func CheckPortOccupied(port int) (int, error) {
	pid, err := getPIDByPort(port)
	if err != nil {
		return -1, fmt.Errorf("检查端口失败: %w", err)
	}
	return pid, nil
}

// TerminateProcessByPort 终止占用指定端口的进程
func TerminateProcessByPort(port int) error {
	pid, err := getPIDByPort(port)
	if err != nil {
		return err
	}

	if pid == -1 {
		log.Printf("[PortManager] 端口 %d 未被占用，无需终止进程", port)
		return nil
	}

	log.Printf("[PortManager] 发现端口 %d 被 PID %d 占用，准备终止...", port, pid)

	cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("终止进程失败: %w, 输出: %s", err, out.String())
	}

	log.Printf("[PortManager] 成功终止 PID %d (端口 %d)", pid, port)
	return nil
}

// getPIDByPort 获取监听在指定端口上的进程 PID
// 如果端口未被占用，返回 -1
func getPIDByPort(port int) (int, error) {
	cmd := exec.Command("netstat", "-ano")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return -1, fmt.Errorf("执行 netstat 失败: %w", err)
	}

	lines := strings.Split(out.String(), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		localAddr := fields[1]
		pidStr := fields[len(fields)-1]

		lastColonIdx := strings.LastIndex(localAddr, ":")
		if lastColonIdx == -1 {
			continue
		}

		addrPort := localAddr[lastColonIdx+1:]
		if addrPort == strconv.Itoa(port) {
			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}
			return pid, nil
		}
	}

	return -1, nil
}
