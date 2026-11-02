package recorder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCleanOld 验证按保留天数清理录制文件：
// 超期删除、未超期保留、非录制命名不动。
func TestCleanOld(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("20060102")
	old := time.Now().AddDate(0, 0, -40).Format("20060102")
	edge := time.Now().AddDate(0, 0, -29).Format("20060102") // 保留期内（cutoff 为 -30，避免跨午夜 flaky 用 -29）

	for _, name := range []string{
		"llm-record-" + old + ".jsonl",
		"llm-record-" + edge + ".jsonl",
		"llm-record-" + today + ".jsonl",
		"notes.txt",
		"llm-record-broken.jsonl",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed := CleanOld(dir, 30)
	if len(removed) != 1 || removed[0] != "llm-record-"+old+".jsonl" {
		t.Fatalf("应只删除 40 天前的文件，实际: %v", removed)
	}
	for _, keep := range []string{
		"llm-record-" + edge + ".jsonl",
		"llm-record-" + today + ".jsonl",
		"notes.txt",
		"llm-record-broken.jsonl",
	} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Fatalf("不应删除 %s: %v", keep, err)
		}
	}

	// keepDays 无效时不做任何清理
	if r := CleanOld(dir, 0); r != nil {
		t.Fatalf("keepDays=0 应返回 nil，实际: %v", r)
	}
	if r := CleanOld(filepath.Join(dir, "不存在"), 30); r != nil {
		t.Fatalf("目录不存在应返回 nil，实际: %v", r)
	}
}

// TestDeleteActiveFile 验证删除「正在被录制（句柄打开）的当天文件」：
// Windows 上直接 os.Remove 会因 sharing violation 失败；Delete 须先释放句柄再删，
// 且删除后继续 Write 自动重建文件、从头记录。
func TestDeleteActiveFile(t *testing.T) {
	dir := t.TempDir()
	r := New()
	defer r.Close() // 末尾 Write 重建了句柄，不关会导致 TempDir 清理失败
	r.Configure(true, dir)
	now := time.Now()
	r.Write(Entry{Time: now, Dir: "req", Body: "first"})
	path := filepath.Join(dir, "llm-record-"+now.Format("20060102")+".jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("录制文件应已创建: %v", err)
	}

	// 句柄仍打开时删除当天文件
	if err := r.Delete(now.Format("20060102")); err != nil {
		t.Fatalf("删除活动文件失败（句柄未释放）: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("文件应已被删除: %v", err)
	}

	// 删除后继续录制：自动重建文件且只含新记录
	r.Write(Entry{Time: time.Now(), Dir: "resp", Body: "second"})
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("删除后应自动重建文件: %v", err)
	}
	if got := string(raw); got != "{}\n" && !containsJSONLCount(raw, 1) {
		t.Fatalf("重建文件应只含 1 条新记录，实际: %q", got)
	}
}

func containsJSONLCount(raw []byte, n int) bool {
	lines := 0
	for _, b := range raw {
		if b == '\n' {
			lines++
		}
	}
	return lines == n
}
