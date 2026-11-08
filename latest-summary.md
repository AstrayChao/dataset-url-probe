# 数据集域名探活报告

- 生成时间（UTC）：2026-11-08T09:12:13Z
- 运行耗时：3m14s｜并发：200｜单次超时：12s
- 总计：25561｜已探测：25560｜跳过：1
- **成功（2xx/3xx）：13947（54.57%）**｜**可达（有 HTTP 响应）：19273（75.40%）**

## 状态码分布

| 类别 | 数量 |
| --- | ---: |
| 2xx | 13894 |
| 3xx | 53 |
| 4xx | 5116 |
| 5xx | 210 |
| no_response | 6288 |

## 结果类型 Top 15

| 类型 | 数量 |
| --- | ---: |
| none | 13904 |
| dns_error | 5219 |
| http_4xx | 5116 |
| timeout | 926 |
| http_5xx | 210 |
| conn_refused | 62 |
| too_many_redirects | 43 |
| conn_closed | 28 |
| tls_error | 23 |
| conn_reset | 17 |
| invalid_url | 11 |
| network_unreachable | 1 |
| skipped_private | 1 |

## 失败域名（按数据集数，Top 20）

| 域名 | HTTP | 类型 | 数据集数 |
| --- | ---: | --- | ---: |
| `competitions.codalab.org` | 0 | timeout | 682 |
| `www.image-net.org` | 0 | timeout | 515 |
| `image-net.org` | 0 | timeout | 493 |
| `anonymous.4open.science` | 403 | http_4xx | 417 |
| `nlp.stanford.edu` | 0 | timeout | 306 |
| `www.yelp.com` | 403 | http_4xx | 212 |
| `jmcauley.ucsd.edu` | 0 | timeout | 207 |
| `www.ldc.upenn.edu` | 403 | http_4xx | 181 |
| `storage.googleapis.com` | 400 | http_4xx | 180 |
| `ai.stanford.edu` | 0 | timeout | 178 |
| `data.vision.ee.ethz.ch` | 403 | http_4xx | 157 |
| `www.ukbiobank.ac.uk` | 403 | http_4xx | 151 |
| `www.clips.uantwerpen.be` | 0 | timeout | 149 |
| `scihub.copernicus.eu` | 0 | timeout | 145 |
| `www.yf.io` | 0 | dns_error | 135 |
| `timeseriesclassification.com` | 403 | http_4xx | 126 |
| `webscope.sandbox.yahoo.com` | 0 | dns_error | 117 |
| `www.reddit.com` | 403 | http_4xx | 116 |
| `www.cdc.gov` | 403 | http_4xx | 108 |
| `wordnet.princeton.edu` | 403 | http_4xx | 107 |

> 说明：`success` 表示最终 HTTP 状态为 2xx/3xx；`reachable` 表示收到了任意 HTTP 响应（含 401/403/5xx）。
> 失败原因可能是站点下线、证书错误、DNS 失效，也可能是目标站点封锁数据中心/IP 段。
