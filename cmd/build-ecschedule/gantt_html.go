package main

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

func renderHTML(rows []ganttRow, offsetHours int, tz string) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<title>ecschedule gantt</title>
<style>
:root { --label-w: 320px; --row-h: 20px; }
* { box-sizing: border-box; }
body { margin: 0; font-family: -apple-system, "Helvetica Neue", "Hiragino Kaku Gothic ProN", Meiryo, sans-serif; font-size: 12px; color: #222; }
header { padding: 12px 16px; border-bottom: 1px solid #ddd; position: sticky; top: 0; background: #fff; z-index: 5; }
header h1 { margin: 0 0 4px; font-size: 15px; }
header .meta { color: #666; font-size: 11px; }
.legend { margin-top: 6px; display: flex; flex-wrap: wrap; gap: 4px 12px; }
.legend span { display: inline-flex; align-items: center; gap: 4px; }
.legend i { width: 11px; height: 11px; border-radius: 2px; display: inline-block; }
.chart { padding: 0 16px 40px; }
.axis { display: flex; position: sticky; top: var(--axis-top, 96px); background: #fff; z-index: 4; border-bottom: 1px solid #ccc; }
.axis .label-pad { width: var(--label-w); flex: none; }
.axis .ticks { position: relative; flex: 1; height: 20px; }
.axis .ticks span { position: absolute; transform: translateX(-50%); color: #888; font-size: 10px; top: 4px; }
.row { display: flex; align-items: center; height: var(--row-h); border-bottom: 1px solid #f2f2f2; }
.row:hover { background: #f6fbff; }
.row.disabled { opacity: .4; }
.row .label { width: var(--label-w); flex: none; padding-right: 8px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row .label .desc { color: #999; margin-left: 6px; }
.track { position: relative; flex: 1; height: 100%;
  background-image: repeating-linear-gradient(to right, #eee 0 1px, transparent 1px calc(100% / 24)); }
.track svg { display: block; width: 100%; height: 100%; }
#tip { position: fixed; pointer-events: none; z-index: 10; max-width: 480px; display: none;
  background: #222; color: #fff; padding: 8px 10px; border-radius: 6px; font-size: 11px; line-height: 1.5; box-shadow: 0 2px 10px rgba(0,0,0,.3); }
#tip b { color: #9ad; }
#tip code { background: rgba(255,255,255,.12); padding: 1px 4px; border-radius: 3px; }
</style>
</head>
<body>
`)

	// ヘッダー + 凡例
	fmt.Fprintf(&b, "<header><h1>ecschedule 実行スケジュール</h1>")
	fmt.Fprintf(&b, `<div class="meta">%d ルール / 横軸 = 1 日 24 時間(%s, UTC%+d)。バーにホバーで詳細。日・曜日制約は cron 上は UTC 基準。</div>`,
		len(rows), html.EscapeString(tz), offsetHours)
	b.WriteString(`<div class="legend">`)
	for _, g := range legendGroups(rows) {
		fmt.Fprintf(&b, `<span><i style="background:%s"></i>%s</span>`, groupColor(g), html.EscapeString(g))
	}
	b.WriteString(`</div></header>`)

	b.WriteString(`<div class="chart">`)

	// 時刻軸
	b.WriteString(`<div class="axis"><div class="label-pad"></div><div class="ticks">`)
	for h := 0; h <= 24; h += 2 {
		left := float64(h) / 24.0 * 100.0
		fmt.Fprintf(&b, `<span style="left:%.4f%%">%d</span>`, left, h)
	}
	b.WriteString(`</div></div>`)

	// 各行
	for _, r := range rows {
		color := groupColor(r.group)
		cls := "row"
		if r.rule.Disabled {
			cls += " disabled"
		}
		tip := buildTip(r, tz)
		fmt.Fprintf(&b, `<div class="%s" data-tip="%s">`, cls, html.EscapeString(tip))
		label := html.EscapeString(r.rule.Name)
		if r.rule.Disabled {
			label += " (disabled)"
		}
		fmt.Fprintf(&b, `<div class="label">%s<span class="desc">%s</span></div>`,
			label, html.EscapeString(r.rule.Description))
		b.WriteString(`<div class="track"><svg viewBox="0 0 1440 10" preserveAspectRatio="none">`)
		for _, run := range r.runs {
			w := run[1] - run[0]
			// 細い区間も視認できるよう最小幅を確保
			drawW := w
			if drawW < 3 {
				drawW = 3
			}
			// viewBox(0..1440)をはみ出さないよう、末尾付近では開始位置を左へ寄せる
			x := run[0]
			if x+drawW > 1440 {
				x = 1440 - drawW
			}
			fmt.Fprintf(&b, `<rect x="%d" y="0" width="%d" height="10" fill="%s"/>`, x, drawW, color)
		}
		b.WriteString(`</svg></div></div>`)
	}

	b.WriteString(`</div>`)

	// ツールチップ + スクリプト
	b.WriteString(`<div id="tip"></div>
<script>
(function(){
  var tip = document.getElementById('tip');
  document.querySelectorAll('.row').forEach(function(row){
    row.addEventListener('mousemove', function(e){
      tip.innerHTML = row.getAttribute('data-tip');
      tip.style.display = 'block';
      var x = e.clientX + 14, y = e.clientY + 14;
      var r = tip.getBoundingClientRect();
      if (x + r.width > window.innerWidth) x = e.clientX - r.width - 14;
      if (y + r.height > window.innerHeight) y = e.clientY - r.height - 14;
      tip.style.left = x + 'px';
      tip.style.top = y + 'px';
    });
    row.addEventListener('mouseleave', function(){ tip.style.display = 'none'; });
  });
})();
</script>
</body>
</html>
`)

	return b.String()
}

// buildTip はホバー時に表示する HTML 文字列(data-tip 属性に格納)を作る。
func buildTip(r ganttRow, tz string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b><br>", html.EscapeString(r.rule.Name))
	if r.rule.Description != "" {
		fmt.Fprintf(&b, "%s<br>", html.EscapeString(r.rule.Description))
	}
	fmt.Fprintf(&b, "<b>cron</b> <code>%s</code><br>", html.EscapeString(r.rule.ScheduleExpression))
	fmt.Fprintf(&b, "<b>command</b> <code>%s</code><br>", html.EscapeString(r.rule.Command))
	fmt.Fprintf(&b, "<b>実行時刻(%s)</b> %s<br>", html.EscapeString(tz), html.EscapeString(timesText(r.minutes)))
	fmt.Fprintf(&b, "<b>条件</b> %s", html.EscapeString(r.cron.constraintText()))
	if len(r.rule.Environment) > 0 {
		fmt.Fprintf(&b, "<br><b>environment</b> %s", html.EscapeString(strings.Join(r.rule.Environment, ", ")))
	}
	if r.rule.Disabled {
		b.WriteString("<br><b>disabled</b>")
	}
	return b.String()
}

func legendGroups(rows []ganttRow) []string {
	seen := map[string]bool{}
	var gs []string
	for _, r := range rows {
		if !seen[r.group] {
			seen[r.group] = true
			gs = append(gs, r.group)
		}
	}
	sort.Strings(gs)
	return gs
}
