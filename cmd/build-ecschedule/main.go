package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:   "build-ecschedule",
		Usage:  "a tool to build ecschedule.yaml (https://github.com/Songmu/ecschedule) from rules.yaml and template",
		Action: buildECSchedule,
		Flags: []cli.Flag{
			&cli.PathFlag{Name: "rules", Required: true, Usage: "rules YAML file"},
			&cli.PathFlag{Name: "output", Value: "ecschedule.yaml", Usage: "output file"},
			&cli.PathFlag{Name: "template", Required: true, Usage: "template file"},
			&cli.StringFlag{Name: "region", Value: "ap-northeast-1", Usage: "aws region"},
			&cli.StringFlag{Name: "cluster", Required: true, Usage: "AWS Cluster"},
			&cli.StringFlag{Name: "environment", Value: "sandbox", Usage: "environment"},
		},
	}

	sort.Sort(cli.FlagsByName(app.Flags))

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

type Rule struct {
	Name               string
	Description        string
	ScheduleExpression string `yaml:"scheduleExpression"`
	Command            string
	Environment        []string
	Disabled           bool
}

var ruleScheduleExpressionRegexp = regexp.MustCompile(`\Acron\([0-9,?*/L -]+\)\z`)
var ruleCommandRegexp = regexp.MustCompile(`\A\[(.+)\]\z`)

func (r *Rule) trimAndCheck() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return fmt.Errorf("name is empty")
	}
	r.Description = strings.TrimSpace(r.Description)
	if r.Description == "" {
		return fmt.Errorf("description is empty")
	}
	r.ScheduleExpression = strings.TrimSpace(r.ScheduleExpression)
	if !ruleScheduleExpressionRegexp.MatchString(r.ScheduleExpression) {
		return fmt.Errorf("scheduleExpression is not valid")
	}
	r.Command = strings.TrimSpace(r.Command)
	if !ruleCommandRegexp.MatchString(r.Command) {
		return fmt.Errorf("command is not valid")
	}
	return nil
}

var prefixTemplate = `region: {{ .Region }}
cluster: {{ .Cluster }}
role: ecs-events
rules:
`

type prefix struct {
	Region  string
	Cluster string
}

func stringContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func buildECSchedule(c *cli.Context) error {
	rulesBs, err := os.ReadFile(c.Path("rules"))
	if err != nil {
		return fmt.Errorf("os.ReadFile(%s) failed: %w", c.Path("rules"), err)
	}
	var rules []Rule
	if err := yaml.Unmarshal(rulesBs, &rules); err != nil {
		return fmt.Errorf("yaml.Unmarshal(rules) failed: %w", err)
	}

	tpl, err := os.ReadFile(c.Path("template"))
	if err != nil {
		return fmt.Errorf("os.ReadFile(%s) failed: %w", c.Path("template"), err)
	}

	var b bytes.Buffer
	b.Grow(len(tpl)*len(rules) + len(prefixTemplate))

	pt, err := template.New("template").Parse(prefixTemplate)
	if err != nil {
		return fmt.Errorf("template.New(template).Parse() failed: %w", err)
	}

	pf := prefix{
		Region:  c.String("region"),
		Cluster: c.String("cluster"),
	}

	if err := pt.Execute(&b, pf); err != nil {
		return fmt.Errorf("t.Execute(prefix) failed:  %w", err)
	}

	t, err := template.New("rule").Parse(string(tpl))
	if err != nil {
		return fmt.Errorf("template.New(rule).Parse() failed: %w", err)
	}

	env := c.String("environment")

	ruleNames := make([]string, 0, len(rules))

	for _, r := range rules {
		if err := r.trimAndCheck(); err != nil {
			return fmt.Errorf("trimAndCheckRule failed: %+v, %w", r, err)
		}
		if len(r.Environment) > 0 && !stringContains(r.Environment, env) {
			log.Printf("skipped! rule.Name=%s", r.Name)
			continue
		}
		if stringContains(ruleNames, r.Name) {
			return fmt.Errorf("detect duplicate rule names: %s", r.Name)
		}
		ruleNames = append(ruleNames, r.Name)
		if err := t.Execute(&b, r); err != nil {
			return fmt.Errorf("t.Execute() failed: rule.Name=%s %w", r.Name, err)
		}
	}

	if err := os.WriteFile(c.Path("output"), b.Bytes(), 0600); err != nil {
		return fmt.Errorf("os.WriteFile(%s) failed: %w", c.Path("output"), err)
	}

	return nil
}
