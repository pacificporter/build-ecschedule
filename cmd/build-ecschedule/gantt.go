package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v2"
)

// ganttCommand は rules.yaml を 1 日 24 時間のガントチャート(HTML)として可視化する。
// cron 式は AWS EventBridge 形式(UTC)としてパースし、表示はタイムゾーンオフセット
// (デフォルト JST = +9)に変換する。
var ganttCommand = &cli.Command{
	Name:   "gantt",
	Usage:  "render rules.yaml as a 24h gantt chart (HTML)",
	Action: runGantt,
	Flags: []cli.Flag{
		&cli.PathFlag{Name: "rules", Required: true, Usage: "rules YAML file"},
		&cli.PathFlag{Name: "output", Value: "ecschedule-gantt.html", Usage: "output HTML file"},
		&cli.IntFlag{Name: "offset", Value: 9, Usage: "timezone offset in hours from UTC (JST=9)"},
		&cli.StringFlag{Name: "tz", Value: "JST", Usage: "timezone label for display"},
		&cli.StringFlag{Name: "sort", Value: "time", Usage: "initial row order: time (first fired time), name, or file (rules order)"},
	},
}

func runGantt(c *cli.Context) error {
	rulesBs, err := os.ReadFile(c.Path("rules"))
	if err != nil {
		return fmt.Errorf("os.ReadFile(%s) failed: %w", c.Path("rules"), err)
	}
	var rules []Rule
	if err := yaml.Unmarshal(rulesBs, &rules); err != nil {
		return fmt.Errorf("yaml.Unmarshal(rules) failed: %w", err)
	}

	offsetMin := c.Int("offset") * 60
	rows := make([]ganttRow, 0, len(rules))
	for i, r := range rules {
		r.Name = strings.TrimSpace(r.Name)
		r.Description = strings.TrimSpace(r.Description)
		r.ScheduleExpression = strings.TrimSpace(r.ScheduleExpression)
		r.Command = strings.TrimSpace(r.Command)

		cron, perr := parseCron(r.ScheduleExpression)
		if perr != nil {
			return fmt.Errorf("parseCron(%q) for rule %q failed: %w", r.ScheduleExpression, r.Name, perr)
		}
		minutes := cron.firedMinutes(offsetMin)
		rows = append(rows, ganttRow{
			rule:      r,
			cron:      cron,
			group:     commandBinary(r.Command),
			minutes:   minutes,
			runs:      mergeRuns(minutes),
			fileIndex: i,
		})
	}

	switch c.String("sort") {
	case "file":
		// rules.yaml の記述順のまま
	case "name":
		sort.SliceStable(rows, func(i, j int) bool {
			return rows[i].rule.Name < rows[j].rule.Name
		})
	default: // "time"
		sort.SliceStable(rows, func(i, j int) bool {
			fi, fj := firstOr(rows[i].minutes, 1<<30), firstOr(rows[j].minutes, 1<<30)
			if fi != fj {
				return fi < fj
			}
			return rows[i].rule.Name < rows[j].rule.Name
		})
	}

	sortMode := c.String("sort")
	if sortMode != "name" && sortMode != "file" {
		sortMode = "time"
	}
	out := renderHTML(rows, c.Int("offset"), c.String("tz"), sortMode)
	if err := os.WriteFile(c.Path("output"), []byte(out), 0600); err != nil {
		return fmt.Errorf("os.WriteFile(%s) failed: %w", c.Path("output"), err)
	}
	fmt.Printf("wrote %s (%d rules)\n", c.Path("output"), len(rows))
	return nil
}

type ganttRow struct {
	rule      Rule
	cron      cronExpr
	group     string
	minutes   []int    // 発火する分(JST, 0..1439)昇順・重複なし
	runs      [][2]int // 連続区間 [start, end)(0..1440)
	fileIndex int      // rules.yaml 上の記述順
}

// ---- cron parsing -----------------------------------------------------------

// cronExpr は AWS EventBridge の cron(分 時 日 月 曜日 年)を保持する。
type cronExpr struct {
	minute, hour, dom, month, dow, year string
}

func parseCron(expr string) (cronExpr, error) {
	s := strings.TrimSpace(expr)
	if !strings.HasPrefix(s, "cron(") || !strings.HasSuffix(s, ")") {
		return cronExpr{}, fmt.Errorf("not a cron() expression")
	}
	body := strings.TrimSuffix(strings.TrimPrefix(s, "cron("), ")")
	f := strings.Fields(body)
	if len(f) != 6 {
		return cronExpr{}, fmt.Errorf("expected 6 fields, got %d", len(f))
	}
	return cronExpr{minute: f[0], hour: f[1], dom: f[2], month: f[3], dow: f[4], year: f[5]}, nil
}

// firedMinutes は 1 日の発火時刻(分単位)を、タイムゾーンオフセットを足した表示時刻
// (0..1439, 翌日へ回り込む場合は剰余)として返す。日・月・曜日の制約は時刻には影響
// しないため、ここでは分・時のみ展開する。
func (e cronExpr) firedMinutes(offsetMin int) []int {
	minutes := expandField(e.minute, 0, 59)
	hours := expandField(e.hour, 0, 23)
	set := make(map[int]bool, len(minutes)*len(hours))
	for _, h := range hours {
		for _, m := range minutes {
			t := ((h*60+m+offsetMin)%1440 + 1440) % 1440
			set[t] = true
		}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// expandField は cron の 1 フィールドを [min, max] の範囲で数値展開する。
// 対応: *, ?, a, a-b, a-b/step, */step, a,b,c, および a>b の巻き戻し範囲。
// L など数値以外のトークンは時刻計算では無視する(制約表示で別途扱う)。
func expandField(field string, min, max int) []int {
	field = strings.TrimSpace(field)
	if field == "*" || field == "?" || field == "" {
		return rangeInts(min, max, 1)
	}
	set := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			if s, err := strconv.Atoi(strings.TrimSpace(part[i+1:])); err == nil && s > 0 {
				step = s
			}
			part = strings.TrimSpace(part[:i])
		}
		var lo, hi int
		switch {
		case part == "*" || part == "?":
			lo, hi = min, max
		case strings.Contains(part, "-"):
			seg := strings.SplitN(part, "-", 2)
			a, err1 := strconv.Atoi(strings.TrimSpace(seg[0]))
			b, err2 := strconv.Atoi(strings.TrimSpace(seg[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			lo, hi = a, b
		default:
			v, err := strconv.Atoi(part)
			if err != nil {
				continue // L などは無視
			}
			lo, hi = v, v
		}
		if lo <= hi {
			for v := lo; v <= hi; v += step {
				if v >= min && v <= max {
					set[v] = true
				}
			}
		} else {
			// 巻き戻し範囲(例: 時の 22-14)
			for v := lo; v <= max; v += step {
				set[v] = true
			}
			for v := min; v <= hi; v += step {
				set[v] = true
			}
		}
	}
	out := make([]int, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

func rangeInts(min, max, step int) []int {
	out := make([]int, 0, (max-min)/step+1)
	for v := min; v <= max; v += step {
		out = append(out, v)
	}
	return out
}

// ---- summaries --------------------------------------------------------------

var jpWeekday = map[int]string{1: "日", 2: "月", 3: "火", 4: "水", 5: "木", 6: "金", 7: "土"}

// constraintText は日・月・曜日フィールドから人間可読の実行条件を組み立てる。
// 注意: EventBridge は日付境界を UTC で評価するため、表示時刻(JST)と曜日がずれる
// 場合がある。ここでは cron に書かれた値をそのまま示す。
func (e cronExpr) constraintText() string {
	var parts []string

	if !isWild(e.dow) {
		days := expandField(e.dow, 1, 7)
		names := make([]string, 0, len(days))
		for _, d := range days {
			if n, ok := jpWeekday[d]; ok {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			parts = append(parts, fmt.Sprintf("曜日: %s (UTC基準)", strings.Join(names, "・")))
		}
	}
	if !isWild(e.dom) {
		parts = append(parts, fmt.Sprintf("日: %s", e.dom))
	}
	if !isWild(e.month) {
		parts = append(parts, fmt.Sprintf("月: %s", e.month))
	}
	if len(parts) == 0 {
		return "毎日"
	}
	return strings.Join(parts, " / ")
}

func isWild(f string) bool {
	f = strings.TrimSpace(f)
	return f == "*" || f == "?" || f == ""
}

// timesText は発火時刻の要約を返す。少数なら HH:MM を列挙し、多い場合は回数で示す。
func timesText(minutes []int) string {
	if len(minutes) == 0 {
		return "(なし)"
	}
	if len(minutes) >= 1440 {
		return "毎分"
	}
	if len(minutes) <= 24 {
		ss := make([]string, 0, len(minutes))
		for _, m := range minutes {
			ss = append(ss, fmt.Sprintf("%02d:%02d", m/60, m%60))
		}
		return strings.Join(ss, ", ")
	}
	return fmt.Sprintf("%d 回/日", len(minutes))
}

// ---- helpers ----------------------------------------------------------------

func commandBinary(command string) string {
	body := strings.TrimSpace(command)
	body = strings.TrimPrefix(body, "[")
	body = strings.TrimSuffix(body, "]")
	for _, tok := range strings.Split(body, ",") {
		tok = strings.TrimSpace(tok)
		tok = strings.Trim(tok, "'\"")
		if tok == "" {
			continue
		}
		if i := strings.LastIndex(tok, "/"); i >= 0 {
			tok = tok[i+1:]
		}
		return tok
	}
	return "(unknown)"
}

func mergeRuns(minutes []int) [][2]int {
	var runs [][2]int
	for _, m := range minutes {
		if len(runs) > 0 && runs[len(runs)-1][1] == m {
			runs[len(runs)-1][1] = m + 1
			continue
		}
		runs = append(runs, [2]int{m, m + 1})
	}
	return runs
}

func firstOr(xs []int, dflt int) int {
	if len(xs) == 0 {
		return dflt
	}
	return xs[0]
}

// groupColor は binary 名から決定的に HSL の色相を割り当てる。
func groupColor(group string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(group); i++ {
		h ^= uint32(group[i])
		h *= 16777619
	}
	hue := int(h % 360)
	return fmt.Sprintf("hsl(%d, 70%%, 45%%)", hue)
}
