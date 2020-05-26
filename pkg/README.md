# Xprober
![image](https://github.com/ning1875/xprober/images/icmp.jpg)
![image](https://github.com/ning1875/xprober/images/http.jpg)


`xprober`  is a distributed c/s architecture interface detection framework:

* Ping monitoring: Based on public cloud hybrid cloud ec2 detection between different regions
* Ping monitoring: Build target pool according to agent startup, can get ping results of two regions as source and destination of each other
* Target Source: At the same time, it also supports the server-side configuration file to specify the target
* Http monitoring: It can get the time spent in different http stages from different regions to the target interface

## Server side logic
- rpc receive agent ip report 
- ticker refresh target into local map 
- rpc give agent its targets 
- rpc receive data
- update to local cache
- ticker data process
- expose prome http metric


## Building
   

```
$ git clone https://github.com/ning1875/xprober.git
# build agent
$ cd  xprober/pkg/cmd/agent && go build -o xprober-agent main.go 
# build server
$ cd ../server/ && go build -o xprober-server main.go 
```


## Start Service
```
# for server 
xprober-server --config.file=xprober.yml
# for agent 
xprober-agent --grpc.server-address=$server_rpc_ip:6001
```

## Integrations with promtheus 
Add the following text to your promtheus.yaml's scrape_configs section
```
# 
scrape_configs:

  - job_name: net_monitor
    honor_labels: true
    honor_timestamps: true
    scrape_interval: 10s
    scrape_timeout: 5s
    metrics_path: /metrics
    scheme: http
    static_configs:
    - targets:
      - $server_rpc_ip:6002
```

## Integrations with grafana
View the metrics names in common/metrics.go and add them to the grafana dashboard
