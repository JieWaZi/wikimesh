GO ?= go
GOCACHE ?= $(CURDIR)/.cache/go-build
BINARY ?= .wikimesh/bin/wikimesh
PACKAGE ?= .wikimesh/dist/wikimesh.tar.gz
LLAMA_LIB ?= .wikimesh/lib
LLAMA_PROCESSOR ?= auto
LLAMA_VERSION ?= latest

.PHONY: test build install-llama package run clean

# 运行普通构建测试；yzma 后端只在运行时需要 .wikimesh/lib 动态库。
test:
	env -u GOROOT GOCACHE=$(GOCACHE) $(GO) test ./...

# 构建默认二进制；本地 GGUF 后端通过 yzma 动态加载 llama.cpp。
build:
	mkdir -p $(dir $(BINARY))
	env -u GOROOT GOCACHE=$(GOCACHE) $(GO) build -o $(BINARY) ./cmd/wikimesh

# 安装 yzma/llama.cpp 运行时动态库，默认目录是 .wikimesh/lib。
install-llama: build
	$(BINARY) model lib install --lib "$(LLAMA_LIB)" --processor "$(LLAMA_PROCESSOR)" --version "$(LLAMA_VERSION)"

# 打包当前构建产物和已安装的 llama.cpp 动态库。
package: build
	mkdir -p $(dir $(PACKAGE))
	rm -rf .wikimesh/package
	mkdir -p .wikimesh/package
	cp "$(BINARY)" .wikimesh/package/
	if [ -d "$(LLAMA_LIB)" ]; then cp -R "$(LLAMA_LIB)"/. .wikimesh/package/; fi
	LANG=C LC_ALL=C tar -czf $(PACKAGE) -C .wikimesh/package .

# 运行默认构建产物。
run: build
	$(BINARY)

# 清理本项目生成的构建产物，不删除已下载模型。
clean:
	rm -rf .wikimesh/bin .wikimesh/dist
