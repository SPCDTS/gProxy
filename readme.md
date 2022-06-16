# gProxy

## 相关配置

* 代理服务端口
    * 18085
* 客户端端口
    * 8888
    * 使用了地址复用与端口复用
    * 是否有必要每个服务一个端口?
* 服务端端口范围:
    * 端口池: 33333-33444
    * 随机选择，随机退避

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
    * 开始转发，代理服务器将开始侦听客户端端口
    * name

* /stop
    * 停止转发
    * name

