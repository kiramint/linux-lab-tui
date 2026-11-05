.PHONY: build build-linux-amd64 build-linux-arm64 build-all test run list clean tidy release-hint

BINARY=linux-lab

build: tidy
	go build -o $(BINARY) .

build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o linux-lab-linux-amd64 .

build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o linux-lab-linux-arm64 .

build-all: build-linux-amd64 build-linux-arm64

tidy:
	go mod tidy

test:
	go test ./...

run: build
	./$(BINARY)

list: build
	./$(BINARY) list

clean:
	rm -f $(BINARY) linux-lab-linux-amd64 linux-lab-linux-arm64

# 正式二进制由 GitHub Actions 发布，勿在本机交叉上传。
# 本机交叉编译可用：make build-linux-amd64 / make build-linux-arm64 / make build-all
# 触发正式 Release：
#   git tag v0.x.y && git push origin v0.x.y
# 或在 Actions 页手动运行 Release workflow（可填 tag）。
release-hint:
	@echo "Local: make build-linux-amd64 | make build-linux-arm64 | make build-all"
	@echo "Release: push a v* tag (or workflow_dispatch) for linux-lab-linux-amd64 / arm64"
	@echo "See .github/workflows/release.yml and GitHub Releases"
