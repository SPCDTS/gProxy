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
    * 注册客户端或服务端，携带form-data，包含以下字段
    * name
    * host
    * port
    * position

* /query
    * 用于内网直连，查询对端ip与端口
    * name, position

* /forwarding
    * 开始转发，代理服务器将开始侦听客户端端口
    * name

* /stop
    * 停止转发
    * name

