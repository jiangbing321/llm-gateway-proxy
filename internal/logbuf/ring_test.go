package logbuf

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRotatingWriterRotation 验证写入路径上的大小轮转：当前文件保持有界，旧文件转 .old。
func TestRotatingWriterRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, err := openRotatingWriter(path, 100)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer w.Close()

	chunk := []byte("0123456789\n") // 11 字节/次
	for i := 0; i < 30; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current: %v", err)
	}
	if st.Size() > 100 {
		t.Fatalf("当前文件超过上限: %d > 100", st.Size())
	}
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Fatalf("未生成 .old 轮转文件: %v", err)
	}

	// 关闭后仍可重新打开继续写（重启场景）
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	w2, err := openRotatingWriter(path, 100)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer w2.Close()
	if _, err := w2.Write(chunk); err != nil {
		t.Fatalf("write after reopen: %v", err)
	}
}

// TestRingFileLogRotate 验证 Ring 落盘路径同样受 maxBytes 约束。
func TestRingFileLogRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "access.log")
	r := NewRing(10)
	if err := r.EnableFileLog(path, 512); err != nil {
		t.Fatalf("enable: %v", err)
	}
	defer r.CloseFileLog()
	for i := 0; i < 200; i++ {
		r.Add(Entry{Path: "/v1/chat/completions", Model: "test-model", ClientIP: "127.0.0.1"})
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// 单条 JSON 约 100+ 字节；512 上限下 200 条必然触发过轮转且当前文件有界
	if st.Size() > 2048 {
		t.Fatalf("访问日志未按上限轮转: %d 字节", st.Size())
	}
	if _, err := os.Stat(path + ".old"); err != nil {
		t.Fatalf("未生成 .old: %v", err)
	}
}
