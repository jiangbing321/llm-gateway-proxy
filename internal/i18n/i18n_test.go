package i18n

import (
	"reflect"
	"sort"
	"testing"
)

// TestKeyParity 校验 zh/en 两份消息表键集合完全一致（防漏译/多译）。
func TestKeyParity(t *testing.T) {
	zhKeys := sortedKeys(messages["zh"])
	enKeys := sortedKeys(messages["en"])
	if !reflect.DeepEqual(zhKeys, enKeys) {
		t.Fatalf("zh/en 键集合不一致:\n仅 zh 有: %v\n仅 en 有: %v",
			diffKeys(zhKeys, enKeys), diffKeys(enKeys, zhKeys))
	}
	if len(zhKeys) == 0 {
		t.Fatal("消息表为空")
	}
}

// TestFallback 校验取词与回退：zh/en 正常取词、带参格式化、双缺回退 key 本身。
func TestFallback(t *testing.T) {
	SetLang("zh")
	defer SetLang("zh")
	if got := T("app.invalidDate"); got != "非法日期" {
		t.Fatalf("zh 取词错误: %q", got)
	}
	SetLang("en")
	if got := T("app.invalidDate"); got != "Invalid date" {
		t.Fatalf("en 取词错误: %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Fatalf("双缺应回退 key 本身: %q", got)
	}
	if got := T("proxy.listenFailed", 8717); got != "Failed to listen on port 8717" {
		t.Fatalf("带参取词错误: %q", got)
	}
}

// TestSetLangNormalize 校验非法值收敛为系统检测结果（zh/en 之一）。
func TestSetLangNormalize(t *testing.T) {
	SetLang("fr")
	if got := Lang(); got != "zh" && got != "en" {
		t.Fatalf("非法语言应收敛为 zh/en，实际 %q", got)
	}
	SetLang("zh")
	if Lang() != "zh" {
		t.Fatalf("SetLang(zh) 失败: %q", Lang())
	}
}

// TestMessagesNonEmpty zh 全键文案非空。
func TestMessagesNonEmpty(t *testing.T) {
	for k, v := range messages["zh"] {
		if v == "" {
			t.Fatalf("zh 键 %q 文案为空", k)
		}
	}
	for k, v := range messages["en"] {
		if v == "" {
			t.Fatalf("en 键 %q 文案为空", k)
		}
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func diffKeys(a, b []string) []string {
	set := map[string]bool{}
	for _, k := range b {
		set[k] = true
	}
	var out []string
	for _, k := range a {
		if !set[k] {
			out = append(out, k)
		}
	}
	return out
}
