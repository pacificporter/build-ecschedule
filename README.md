# build-ecschedule

build-ecschedule is a tool to build ecschedule.yaml ([Songmu/ecschedule: ecschedule is a tool to manage ECS Scheduled Tasks.](https://github.com/Songmu/ecschedule)) from a rules file and a template files.

# Usage

```console
% go install github.com/pacificporter/build-ecschedule/cmd/build-ecschedule@latest
% build-ecschedule --rules sample/ecschedule.rules.yaml --template sample/ecschedule.rule.yaml.template --environment production --cluster tokyo-production --output ecschedule.yaml
```

## gantt

`gantt` サブコマンドは rules.yaml を「1 日 24 時間のガントチャート」として HTML に可視化します。どのバッチがいつ実行されるかを俯瞰できます。

```console
% build-ecschedule gantt --rules sample/ecschedule.rules.yaml --output ecschedule-gantt.html
```

- cron 式は AWS EventBridge 形式(UTC)としてパースし、表示は `--offset`(デフォルト 9 = JST)で変換します。
- 各行はバーにホバーすると cron / command / 実行時刻 / 日・曜日などの条件を表示します。
- 行は実行開始時刻順に並びます。`--sort file` で rules.yaml の記述順になります。
- バーの色は command のバイナリ名ごとに割り当てます。

| flag | default | 説明 |
|------|---------|------|
| `--rules` | (必須) | rules YAML ファイル |
| `--output` | `ecschedule-gantt.html` | 出力 HTML ファイル |
| `--offset` | `9` | UTC からのオフセット時間(JST=9) |
| `--tz` | `JST` | 表示用タイムゾーンラベル |
| `--sort` | `time` | 行の並び順(`time` = 実行開始時刻順 / `file` = 記述順) |
