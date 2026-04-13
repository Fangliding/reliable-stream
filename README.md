# reliable-stream

个人用于研究 gvisor 接口的小玩具

基于 gvisor 的用户态 TCP/IP 栈，在任意不可靠传输上构建可靠双向流。

接口使用极其简单，只需要在客户端提供一个不可靠的 `io.ReadWriteCloser` 并且在服务端注册它对端的 `io.ReadWriteCloser`，就可以重建可靠流。服务端可以自动处理来自多个 io 的客户端连接，客户端可以多路复用打开多个 stream。不绑定 UDP，任何不可靠 io 都行，UDP，无法以双工进行的 HTTP POST，信鸽，飞路网，只要不丢包边界。其实我很好奇网上似乎没有类似物，要么就是和 UDP 强相关。

demo.go 里有一个简单的 demo，只是从 udp conn 里进行了简单的 demux 就可以接入了。服务端和客户端之间使用 UDP 通讯中继 TCP 连接。