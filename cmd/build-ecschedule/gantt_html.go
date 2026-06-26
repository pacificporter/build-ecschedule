package main

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

func renderHTML(rows []ganttRow, offsetHours int, tz, sortMode string) string {
	var b strings.Builder

	b.WriteString(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="utf-8">
<title>ecschedule gantt</title>
<style>
:root { --label-w: 320px; --row-h: 34px; }
* { box-sizing: border-box; }
body { margin: 0; font-family: -apple-system, "Helvetica Neue", "Hiragino Kaku Gothic ProN", Meiryo, sans-serif; font-size: 12px; color: #222; }
/* ヘッダーと時刻軸をひとつの sticky にまとめ、行がその下にスクロールするようにする
   (個別 sticky + 固定 top だとヘッダー高とずれて先頭行が隠れるため) */
.topbar { position: sticky; top: 0; z-index: 5; background: #fff; border-bottom: 1px solid #ccc; }
header { padding: 12px 16px; }
header h1 { margin: 0 0 4px; font-size: 15px; }
header .meta { color: #666; font-size: 11px; }
.toolbar { margin-top: 8px; display: flex; align-items: center; gap: 10px; }
.toolbar input { width: 320px; max-width: 60vw; padding: 4px 8px; font-size: 12px; border: 1px solid #ccc; border-radius: 4px; }
.toolbar label { color: #666; font-size: 11px; }
.toolbar select { font-size: 12px; padding: 3px 4px; }
.toolbar .count { color: #666; font-size: 11px; }
.legend { margin-top: 6px; display: flex; flex-wrap: wrap; gap: 4px 12px; }
.legend span { display: inline-flex; align-items: center; gap: 4px; cursor: pointer; user-select: none; }
.legend span.off { opacity: .35; text-decoration: line-through; }
.legend i { width: 11px; height: 11px; border-radius: 2px; display: inline-block; }
.row.hidden { display: none; }
.chart { padding: 0 16px 40px; }
.axis { display: flex; padding: 4px 16px; border-top: 1px solid #eee; }
.axis .label-pad { width: var(--label-w); flex: none; }
.axis .ticks { position: relative; flex: 1; height: 16px; }
.axis .ticks span { position: absolute; transform: translateX(-50%); color: #888; font-size: 10px; top: 2px; }
.row { display: flex; align-items: center; height: var(--row-h); border-bottom: 1px solid #f2f2f2; }
.row:hover { background: #f6fbff; }
.row.disabled { opacity: .4; }
.row .label { width: var(--label-w); flex: none; padding-right: 8px; display: flex; flex-direction: column; justify-content: center; overflow: hidden; }
.row .label .name { font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row .label .desc { color: #999; font-size: 10px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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

	// ヘッダー + 凡例 + 時刻軸を 1 つの sticky(.topbar)にまとめる
	b.WriteString(`<div class="topbar">`)
	fmt.Fprintf(&b, "<header><h1>ecschedule 実行スケジュール</h1>")
	fmt.Fprintf(&b, `<div class="meta">%d ルール / 横軸 = 1 日 24 時間(%s, UTC%+d)。バーにホバーで詳細。日・曜日制約は cron 上は UTC 基準。</div>`,
		len(rows), html.EscapeString(tz), offsetHours)
	b.WriteString(`<div class="toolbar">`)
	b.WriteString(`<input id="search" type="search" placeholder="名前・説明・コマンドで絞り込み" autocomplete="off">`)
	b.WriteString(`<label>並び替え <select id="sort">`)
	for _, opt := range []struct{ val, label string }{
		{"time", "実行時刻順"},
		{"name", "バッチ名順"},
		{"file", "記述順"},
	} {
		sel := ""
		if opt.val == sortMode {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, opt.val, sel, opt.label)
	}
	b.WriteString(`</select></label>`)
	b.WriteString(`<span class="count" id="count"></span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="legend" title="クリックで表示/非表示を切り替え">`)
	for _, g := range legendGroups(rows) {
		fmt.Fprintf(&b, `<span class="lg" data-group="%s"><i style="background:%s"></i>%s</span>`,
			html.EscapeString(g), groupColor(g), html.EscapeString(g))
	}
	b.WriteString(`</div></header>`)

	// 時刻軸
	b.WriteString(`<div class="axis"><div class="label-pad"></div><div class="ticks">`)
	for h := 0; h <= 24; h += 2 {
		left := float64(h) / 24.0 * 100.0
		fmt.Fprintf(&b, `<span style="left:%.4f%%">%d</span>`, left, h)
	}
	b.WriteString(`</div></div>`)
	b.WriteString(`</div>`) // .topbar

	b.WriteString(`<div class="chart" id="chart">`)

	// 各行
	for _, r := range rows {
		color := groupColor(r.group)
		cls := "row"
		if r.rule.Disabled {
			cls += " disabled"
		}
		tip := buildTip(r, tz)
		search := strings.ToLower(r.rule.Name + " " + r.rule.Description + " " + r.rule.Command)
		fmt.Fprintf(&b, `<div class="%s" data-group="%s" data-search="%s" data-name="%s" data-first="%d" data-index="%d" data-tip="%s">`,
			cls, html.EscapeString(r.group), html.EscapeString(search),
			html.EscapeString(strings.ToLower(r.rule.Name)), firstOr(r.minutes, 1<<30), r.fileIndex,
			html.EscapeString(tip))
		label := html.EscapeString(r.rule.Name)
		if r.rule.Disabled {
			label += " (disabled)"
		}
		fmt.Fprintf(&b, `<div class="label"><span class="name">%s</span><span class="desc">%s</span></div>`,
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

  // 検索 + 凡例(グループ)フィルタ + 並び替え
  var rows = Array.prototype.slice.call(document.querySelectorAll('.row'));
  var search = document.getElementById('search');
  var count = document.getElementById('count');
  var sortSel = document.getElementById('sort');
  var chart = document.getElementById('chart');
  var offGroups = {};

  function sortRows(){
    var mode = sortSel.value;
    var sorted = rows.slice();
    sorted.sort(function(a, b){
      if (mode === 'name') {
        return a.getAttribute('data-name').localeCompare(b.getAttribute('data-name'));
      }
      if (mode === 'file') {
        return (+a.getAttribute('data-index')) - (+b.getAttribute('data-index'));
      }
      // time: 開始時刻 → 名前
      var d = (+a.getAttribute('data-first')) - (+b.getAttribute('data-first'));
      if (d !== 0) return d;
      return a.getAttribute('data-name').localeCompare(b.getAttribute('data-name'));
    });
    sorted.forEach(function(row){ chart.appendChild(row); });
  }
  sortSel.addEventListener('change', sortRows);
  function apply(){
    var q = (search.value || '').toLowerCase().trim();
    var visible = 0;
    rows.forEach(function(row){
      var g = row.getAttribute('data-group');
      var hay = row.getAttribute('data-search') || '';
      var ok = !offGroups[g] && (q === '' || hay.indexOf(q) >= 0);
      row.classList.toggle('hidden', !ok);
      if (ok) visible++;
    });
    count.textContent = visible + ' / ' + rows.length + ' 件';
  }
  search.addEventListener('input', apply);
  document.querySelectorAll('.legend .lg').forEach(function(el){
    el.addEventListener('click', function(){
      var g = el.getAttribute('data-group');
      offGroups[g] = !offGroups[g];
      el.classList.toggle('off', !!offGroups[g]);
      apply();
    });
  });
  apply();
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
