package stats

import (
	"testing"
	"time"

	"llm-gateway-proxy/internal/paths"
)

// TestGetStatsContinuousDays 验证按天聚合的连续日期轴：
// cutoff..今天 每天一行（无数据为零值），当天数据在最后一行。
// （仅操作内存 cells，GetStats 不落盘，不污染真实数据。）
func TestGetStatsContinuousDays(t *testing.T) {
	if _, err := paths.DataDir(); err != nil {
		t.Skip("数据目录不可用")
	}
	st, err := New()
	if err != nil {
		t.Fatal(err)
	}
	// 清空内存 cells，注入两条：今天 与 5 天前
	st.mu.Lock()
	st.cells = map[string]*Cell{}
	today := time.Now().Format("2006-01-02")
	fiveDaysAgo := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	st.cells[today+"|glm|g1"] = &Cell{Date: today, Provider: "glm", Model: "g1",
		Requests: 3, PromptTokens: 100, CompletionTokens: 50}
	st.cells[fiveDaysAgo+"|glm|g1"] = &Cell{Date: fiveDaysAgo, Provider: "glm", Model: "g1",
		Requests: 1, PromptTokens: 10, CompletionTokens: 5}
	st.mu.Unlock()

	s := st.GetStats(7)
	if len(s.Days) != 7 {
		t.Fatalf("应返回连续 7 天，实际 %d: %+v", len(s.Days), s.Days)
	}
	cutoff := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	if s.Days[0].Date != cutoff || s.Days[6].Date != today {
		t.Fatalf("首末日错误: %s .. %s（应 %s .. %s）", s.Days[0].Date, s.Days[6].Date, cutoff, today)
	}
	// 5 天前的数据落在对应行
	found := false
	for _, d := range s.Days {
		if d.Date == fiveDaysAgo {
			if d.Requests != 1 {
				t.Fatalf("5 天前数据错误: %+v", d)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("缺少 %s 行", fiveDaysAgo)
	}
	// 中间无数据天为零值（today-2 无注入）
	for _, d := range s.Days {
		if d.Date == time.Now().AddDate(0, 0, -2).Format("2006-01-02") &&
			(d.Requests != 0 || d.PromptTokens != 0) {
			t.Fatalf("空白天应零值: %+v", d)
		}
	}
	// 当天数据在最后一行
	last := s.Days[6]
	if last.Requests != 3 || last.PromptTokens != 100 || last.CompletionTokens != 50 {
		t.Fatalf("当天数据错误: %+v", last)
	}
	if s.Total.Requests != 4 {
		t.Fatalf("汇总错误: %+v", s.Total)
	}
	// 明细行：两天同一 provider|model 聚合为一行（回归：曾漏写 sum.Rows 导致明细恒空）
	if len(s.Rows) != 1 {
		t.Fatalf("明细应聚合为 1 行，实际 %d: %+v", len(s.Rows), s.Rows)
	}
	if s.Rows[0].Requests != 4 || s.Rows[0].PromptTokens != 110 {
		t.Fatalf("明细聚合值错误: %+v", s.Rows[0])
	}
}
