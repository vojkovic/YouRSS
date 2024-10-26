all:
	go run main.go

docker:
	docker build -t yourss .
	docker run -p 8080:8080 -v ./config.yaml:/config.yaml yourss

.PHONY: all