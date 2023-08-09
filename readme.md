# gProxy

## 相关配置

* 代理服务端口:18085
* 服务端端口范围: 33333-33444

## 使用

* /register

  * 注册服务端，携带form-data，包含以下字段
  * name
  * host
  * port
* /query

  * 携带参数:
    * mode
      * mode="direct"表示直连, 将返回对端IP和端口
      * mode="proxy"表示代理, 将返回代理服务器IP和端口
    * name
* /forwarding

  * 开始转发，代理服务器将建立与服务端的连接，同时开始侦听客户端端口
  * name
* /stop

  * 停止转发
  * name

## 简介

* 放在有外部IP的跳板机上，将发送到外部IP+端口的tcp连接转发到注册过的服务端
* 基于Reactor模型
* 使用Epoll进行IO多路复用
* 一个Epoll池用于监听新连接、一个Epoll池用于发起连接和处理可读可写事件

## 进化史

1. 来一个连接，创建一对goroutine去搬运数据，一个s->p->c，另一个c->p->s，ab压测c500直接上千个goroutine。
2. 使用通过channel来同步的协程池，长连接会一直占用协程，工作队列再长可用的协程数也会一直减少，最终导致所有协程都在accept，ab压测c500、n10000看不出啥问题，但有隐患。
3. 基于Epoll的Reactor模型，固定数目的协程进入EpollWait系统调用，多路复用就是好。
