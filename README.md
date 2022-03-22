# build-ecschedule

build-ecschedule is a tool to build ecschedule.yaml ([Songmu/ecschedule: ecschedule is a tool to manage ECS Scheduled Tasks.](https://github.com/Songmu/ecschedule)) from a rules file and a template files.

# Usage

```console
% go install github.com/pacificporter/build-ecschedule/cmd/build-ecschedule@latest
% build-ecschedule --rules sample/ecschedule.rules.yaml --template sample/ecschedule.rule.yaml.template --environment production --cluster tokyo-production --output ecschedule.yaml
```
