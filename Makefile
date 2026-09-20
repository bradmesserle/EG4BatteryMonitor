main_go_path=./eg4-monitor
build_dir=./bin
binary_name=eg4-monitor

.REST_API: build

all: update-dependencies compile-templ build

update-dependencies:
	go mod tidy

compile-templ:
	go tool templ generate


build:
	@mkdir -p ${build_dir}
	go mod tidy
	GOARCH=arm64 GOOS=linux go build -o ${build_dir}/${binary_name}-arm64 ${main_go_path}