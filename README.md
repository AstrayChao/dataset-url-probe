# 数据集域名探活（dataset-url-probe）

每天批量探测数据集站点域名的可访问性，输出带 **HTTP 状态码** 和 **是否成功** 的 CSV。
输入是 `data/domains.csv`（来自《数据集URL域名统计.xlsx》的 25,561 个域名，全部不带 `http://` / `https://`）。

- 单文件 Go 二进制，零运行时依赖，交叉编译即可上服务器
- 并发探活；域名没有协议前缀时自动先试 `https://`，网络层失败再回退 `http://`
- **默认忽略 TLS 证书校验**：证书过期、自签、域名不匹配都仍然算“可访问”（可用 `-tls-verify` 打开严格校验）
- 手动跟随重定向（最多 5 跳）并记录最终 URL
- 输出 UTF-8 BOM CSV（Excel 直接打开不乱码）+ JSON/Markdown 汇总
- GitHub Actions 每天定时跑，用 GitHub 海外机房出口模拟“国外源”访问，生成可下载的 CSV 产物
- 结果同步发布到 `results` 分支，可直接用 raw 链接下载最新 CSV

## 判定口径

| 字段 | 含义 |
| --- | --- |
| `http_code` | 最终 HTTP 状态码；没有任何响应时为 `0` |
| `success` | `true` = 最终状态码为 2xx/3xx（跟随重定向后的最终页面） |
| `reachable` | `true` = 收到了任意 HTTP 响应（包括 401/403/429/5xx） |
| `error_kind` | 失败类型：`dns_error` / `timeout` / `tls_error` / `conn_refused` / `http_4xx` / `http_5xx` 等 |
| `latency_ms` | 本次探测总耗时（含 https→http 回退和重定向） |
| `attempts` | 实际尝试过的协议，如 `https`、`https,http` |

> 探活目标是域名的根路径 `/`。源表只有域名，没有每条数据集的完整 URL，因此这里衡量的是“站点是否可访问”，不是某条数据集链接是否有效。

## 快速开始（本地）

```bash
# 需要 Go 1.22+
make build                # 或 go build -o bin/dataset-probe ./cmd/probe

./bin/dataset-probe -input data/domains.csv -outdir out
```

输出：

```
out/
├── results.csv     # 逐域名结果，25,561 行
├── summary.json    # 机器可读汇总
└── summary.md      # 人读汇总（GitHub Actions 也贴到 Step Summary）
```

先小样本试跑：

```bash
./bin/dataset-probe -input data/domains.csv -outdir out_smoke -limit 200
```

### 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-input` | 必填 | CSV / TSV / TXT，第一列或每行为域名或 URL |
| `-outdir` | `out` | 输出目录 |
| `-concurrency` | `200` | 并发数 |
| `-timeout` | `12s` | 单个域名单次尝试总超时（含重定向） |
| `-retries` | `0` | 网络层错误重试次数 |
| `-max-redirects` | `5` | 最大重定向次数 |
| `-tls-verify` | `false` | 严格校验 TLS 证书；默认忽略证书问题 |
| `-skip-private` | `true` | 跳过私有/回环/链路本地字面 IP（如 `10.9.20.10:8080`） |
| `-dated` | `false` | 输出文件名带日期，便于服务器保留历史 |
| `-max-duration` | `0` | 整轮最长耗时，0 不限制 |
| `-limit` | `0` | 只跑前 N 条，0 为全部 |
| `-user-agent` | 桌面 Chrome | 请求 UA |
| `-quiet` | `false` | 不打印进度 |

### 输入格式

带表头的 CSV（当前数据）：

```csv
domain,dataset_count,url_occurrences,distinct_urls,usage_records
github.com,55420,150463,62422,149803
huggingface.co,5177,8851,4616,8773
```

也支持纯文本，一行一个域名/URL：

```
github.com
https://huggingface.co/
archive.ics.uci.edu/dataset/...
```

解析时会自动处理：缺省协议、大小写、端口、IDN（`doñasol.com` → punycode）、尾随空格。

## 部署到服务器（二进制）

```bash
# 1. 交叉编译 Linux 二进制（在任意有 Go 的机器上）
make linux-amd64            # 产物 dist/dataset-probe-linux-amd64

# 2. 上传
sudo mkdir -p /opt/dataset-probe/data /var/lib/dataset-probe
sudo useradd -r -s /usr/sbin/nologin datasetprobe
sudo cp dist/dataset-probe-linux-amd64 /opt/dataset-probe/dataset-probe
sudo cp data/domains.csv /opt/dataset-probe/data/
sudo chown -R datasetprobe:datasetprobe /var/lib/dataset-probe
sudo chmod +x /opt/dataset-probe/dataset-probe

# 3. systemd 定时任务
sudo cp deploy/dataset-probe.service deploy/dataset-probe.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now dataset-probe.timer

# 4. 查看
systemctl list-timers dataset-probe.timer
journalctl -u dataset-probe.service -n 100
ls -lh /var/lib/dataset-probe/
```

`dataset-probe.service` 默认带 `-dated`，每天生成 `results-YYYY-MM-DD.csv`，不会覆盖历史。
如果不用 systemd，`deploy/crontab.example` 里有 `flock` + cron 的写法。

也可以直接跑 Docker：

```bash
docker build -t dataset-probe .
docker run --rm -v "$PWD/data:/data:ro" -v "$PWD/out:/out" dataset-probe \
  -input /data/domains.csv -outdir /out
```

## GitHub Actions（海外源）

### 一次性配置

```bash
git init -b main
git add .
git commit -m "init: dataset url probe"
gh repo create <owner>/<repo> --public --source=. --remote=origin --push
```

推送后：

1. 到 **Actions → daily-probe → Run workflow**，先用 `limit=200` 试跑。
2. 全量跑完（约 10–30 分钟）后，在 **Actions → 对应 run → Artifacts** 下载
   `probe-results-*.zip`（CSV / JSON / Markdown，保留 90 天）。
3. 同时在 `results` 分支生成稳定下载地址：

```
https://raw.githubusercontent.com/<owner>/<repo>/results/latest.csv
https://raw.githubusercontent.com/<owner>/<repo>/results/latest-summary.json
https://raw.githubusercontent.com/<owner>/<repo>/results/latest-summary.md
https://raw.githubusercontent.com/<owner>/<repo>/results/history/daily_summary.csv
```

### 定时任务

- cron：`17 2 * * *`（UTC，北京时间 10:17），可在 `.github/workflows/probe.yml` 里改
- 运行在 `ubuntu-latest`（GitHub 托管，Azure 海外机房），即“国外源”出口
- Step Summary 会附上 runner 信息和 `ipinfo.io` 查询到的出口 IP/地区，便于确认来源
- `results` 分支每次 force push 单提交，只保留最新 CSV + 每日汇总，仓库不会每天膨胀
- Artifact 保留 90 天；完整历史请自行下载归档

> 注意：GitHub 公共仓库若连续 60 天没有任何提交活动，定时工作流会被自动停用，需要到 Actions 页面重新启用。

### 国外源与本地源的差异

GitHub 机房出口和你本地网络看到的结果会不同，这是正常的：

- 站点屏蔽数据中心 IP / 地域限制 → 403、429、超时
- 证书过期 / 自签 → 已默认忽略，不会误判为不可访问
- CDN 按地区返回不同节点 → 状态码和延迟可能不同
- 某些学术站点只允许校园网访问 → 会显示 403/超时

建议把 Actions 的结果当作“海外公网可达性”，本地/服务器结果当作“国内可达性”对照。

## 更新域名清单

源表更新后重新生成：

```bash
pip install openpyxl
python tools/extract_domains.py \
  "data/source/数据集URL域名统计.xlsx" \
  data/domains.csv
```

## 结果字段

| 列 | 说明 |
| --- | --- |
| `domain` | 原始输入值 |
| `url` | 最后一次实际尝试的 URL（含协议） |
| `http_code` | 最终 HTTP 状态码，`0` = 无响应 |
| `success` | 2xx/3xx 为 `true` |
| `reachable` | 收到任意 HTTP 响应为 `true` |
| `scheme` | 最终使用的协议：`https` / `http` |
| `attempts` | 尝试过的协议序列 |
| `final_url` | 跟随重定向后的最终 URL |
| `redirects` | 重定向次数 |
| `error_kind` | 失败/跳过类型 |
| `error` | 原始错误信息（截断 300 字符） |
| `latency_ms` | 总耗时毫秒 |
| `dataset_count` 等 4 列 | 从源表透传的统计值，便于按影响面排序 |
| `checked_at` | 探测完成时间（UTC） |

常用筛选：

```bash
# 成功
awk -F, 'NR==1 || $4=="true"' out/results.csv > out/success.csv
# 收到响应但失败（403/404/5xx 等）
awk -F, 'NR==1 || ($5=="true" && $4=="false")' out/results.csv > out/http_errors.csv
# 完全没有响应
awk -F, 'NR==1 || $5=="false"' out/results.csv > out/unreachable.csv
```

## 开发

```bash
go test ./...
go vet ./...
gofmt -l cmd
```

## License

MIT
