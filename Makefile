default:binaries

clean:
	@echo "Cleaning binaries"
	@rm -rf bin/ ./owns

test:
	GOOS=linux   GOARCH=amd64 go build -o bin/owns
	./bin/owns -confDir ./conf -logLevel DEBUG -port 1053


binaries:clean
	GOOS=linux   GOARCH=amd64 go build -o bin/owns
	GOOS=linux   GOARCH=arm64 go build -o bin/owns-arm64
	GOOS=netbsd  GOARCH=amd64 go build -o bin/owns-netbsd
	GOOS=windows GOARCH=amd64 go build -o bin/owns.exe
	GOOS=darwin  GOARCH=amd64 go build -o bin/owns-darwin

install:
	cp -r conf/ /etc/owns
	cp owns.service /usr/lib/systemd/system/owns.service
	cp bin/owns /usr/local/bin
	
