default:build

clean:
	@echo "Cleaning binaries"
	@rm -rf bin/

test:
	GOOS=linux   GOARCH=amd64 go build -o bin/owns-linux64
	./bin/owns-linux64 -confDir ./conf -logLevel DEBUG -port 1053


build:
	GOOS=linux   GOARCH=amd64 go build -o bin/owns-linux64

binaries:clean
	GOOS=linux   GOARCH=amd64 go build -o bin/owns-linux64
	GOOS=linux   GOARCH=arm64 go build -o bin/owns-arm64
	GOOS=netbsd  GOARCH=amd64 go build -o bin/owns-netbsd
	GOOS=windows GOARCH=amd64 go build -o bin/owns.exe
	GOOS=darwin  GOARCH=amd64 go build -o bin/owns-darwin

install:build
	cp -r conf/ /etc/owns
	cp owns.service /usr/lib/systemd/system/owns.service
	cp bin/owns-linux64 /usr/local/bin/owns
	
reinstall:build
	systemctl stop owns
	cp bin/owns-linux64 /usr/local/bin/owns
	systemctl start owns
	systemctl status owns