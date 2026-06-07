package embed

import "os/exec"

// newEmbeddingCommand 封装外部 embedding 命令创建，便于测试和 yzma 默认路径分离。
func newEmbeddingCommand(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
