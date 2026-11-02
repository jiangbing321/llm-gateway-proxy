//go:build windows

// Package singleinstance 提供单实例互斥 + 二次启动时唤醒已运行实例的能力。
package singleinstance

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	mutexName = `Global\LLMGatewayProxy.SingleInstance`
	eventName = `Global\LLMGatewayProxy.ShowWindow`
)

type Instance struct {
	mu    sync.Mutex
	mutex windows.Handle
	event windows.Handle // 唤醒事件（仅主实例持有）
}

// Acquire 尝试成为唯一实例。
// ok=false 表示已有实例在运行（此时已发出唤醒信号，调用方应立即退出）。
func Acquire() (inst *Instance, ok bool) {
	handle, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").
		Call(0, 1, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(mutexName))))
	if handle == 0 {
		// 创建失败（极端）：放行，退化为允许多实例
		return &Instance{}, true
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		signalExisting()
		return nil, false
	}
	inst = &Instance{mutex: windows.Handle(handle)}
	if evt, eerr := createEvent(); eerr == nil {
		inst.event = evt
	}
	return inst, true
}

// createEvent 创建/打开自动复位的命名事件。
func createEvent() (windows.Handle, error) {
	r, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateEventW").
		Call(0, 0, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(eventName))))
	if r == 0 {
		return 0, err
	}
	return windows.Handle(r), nil
}

// signalExisting 通知已运行实例显示主窗口（EVENT_MODIFY_STATE 打开 + SetEvent）。
func signalExisting() {
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("OpenEventW").
		Call(0x2, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(eventName))))
	if r == 0 {
		return
	}
	defer windows.CloseHandle(windows.Handle(r))
	syscall.NewLazyDLL("kernel32.dll").NewProc("SetEvent").Call(r)
}

// WaitShowSignal 阻塞等待唤醒信号；句柄无效（创建失败）返回 false。
// 由主实例在独立 goroutine 中循环调用。
func (i *Instance) WaitShowSignal() bool {
	if i.event == 0 {
		return false
	}
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("WaitForSingleObject").
		Call(uintptr(i.event), windows.INFINITE)
	return r == 0 // WAIT_OBJECT_0
}

// Release 释放句柄（进程退出时调用）。
func (i *Instance) Release() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.event != 0 {
		windows.CloseHandle(i.event)
		i.event = 0
	}
	if i.mutex != 0 {
		windows.CloseHandle(i.mutex)
		i.mutex = 0
	}
}
