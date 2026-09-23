main_go_path=./eg4-monitor
build_dir=./bin
binary_name=eg4-monitor
version=0.0.1

.REST_API: build

all: fetch-dependencies build

fetch-dependencies:
	go mod download github.com/a-h/templ
	go get github.com/a-h/templ/parser/v2@v0.3.1020
	go get github.com/a-h/templ/cmd/templ/generatecmd@v0.3.1020
	go get github.com/a-h/templ/internal/imports@v0.3.1020
	go get github.com/a-h/templ/cmd/templ/fmtcmd@v0.3.1020
	go tool templ generate
	go mod tidy

compile-templ:
	go tool templ generate


build:
	@mkdir -p ${build_dir}
	go mod tidy
	GOARCH=arm64 GOOS=linux go build -o ${build_dir}/${binary_name}-arm64 ${main_go_path}


package-deb:
	@mkdir -p /tmp/EG4-Monitor-${version}_arm64/DEBIAN
	@mkdir -p /tmp/EG4-Monitor-${version}_arm64/opt/eg4
	cp ${build_dir}/${binary_name}-arm64 /tmp/EG4-Monitor-${version}_arm64/opt/eg4
	cp unix-scripts/systemd/eg4-battery-monitor.service /tmp/EG4-Monitor-${version}_arm64/opt/eg4
	chmod +x /tmp/EG4-Monitor-${version}_arm64/opt/eg4
	cp deb-package-files/control /tmp/EG4-Monitor-${version}_arm64/DEBIAN
	cp deb-package-files/postinst /tmp/EG4-Monitor-${version}_arm64/DEBIAN
	cp deb-package-files/postrm /tmp/EG4-Monitor-${version}_arm64/DEBIAN
	chmod +x  /tmp/EG4-Monitor-${version}_arm64/DEBIAN/postinst
	chmod +x  /tmp/EG4-Monitor-${version}_arm64/DEBIAN/postrm
	dpkg-deb --build --root-owner-group /tmp/EG4-Monitor-${version}_arm64