.PHONY: build test run list clean tidy release-hint

BINARY=linux-lab

build: tidy
	go build -o $(BINARY) .

tidy:
	go mod tidy

test:
	go test ./...

run: build
	./$(BINARY)

list: build
	./$(BINARY) list

clean:
	rm -f $(BINARY)

# 正式二进制由 GitHub Actions 发布，勿在本机交叉上传。
# 触发方式：
#   git tag v0.x.y && git push origin v0.x.y
# 或在 Actions 页手动运行 Release workflow（可填 tag）。
release-hint:
	@echo "Push a v* tag (or run Release via workflow_dispatch) to build linux-lab-linux-amd64 / arm64"
	@echo "See .github/workflows/release.yml and GitHub Releases"
