.PHONY: test
test:
	go build -o build-ecschedule ./cmd/build-ecschedule
	./build-ecschedule --rules sample/ecschedule.rules.yaml --template sample/ecschedule.rule.yaml.template --environment production --cluster tokyo-production --output test.yaml
	diff -u sample/ecschedule.yaml test.yaml
	rm build-ecschedule test.yaml
