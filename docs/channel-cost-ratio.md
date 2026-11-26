# 渠道级成本倍率（channel_cost_ratio）

基于 `v1.0.0-rc.47` 的最小侵入补丁：让同一个分组下的多条渠道各自携带结算倍率，从而实现「一个分组、一个对外价格、多条上游按成本各自结算」。

## 为什么需要它

New API 的定价是 `ModelRatio（全局）× GroupRatio（分组）`。

这意味着：同一个模型放在同一个分组里，无论实际由哪条渠道服务，结算价格完全一样。想按上游成本分开计价，只能给每个上游单独开分组，于是分组数量随上游数量线性膨胀，用户也被迫理解一堆内部线路名。

这个补丁在结算时引入第三层：`ChannelCostRatio（渠道）`。三条渠道可以同属一个分组，对外价格不变，但每次请求按实际命中的渠道结算。

## 改动点

| 文件 | 改动 |
|---|---|
| `relaykit/dto/channel_settings.go` | `ChannelSettings` 新增 `channel_cost_ratio` 字段与 `EffectiveCostRatio()` |
| `types/price_data.go` | `GroupRatioInfo` 新增 `ChannelCostRatio`，便于日志与观测 |
| `relay/helper/price.go` | `HandleGroupRatio` 把渠道倍率折进 `GroupRatio` |
| `controller/channel.go` | 更新渠道时保留客户端未提交的 `channel_cost_ratio`，避免后台表单保存把它清空 |

折算方式：`实际倍率 = GroupRatio × ChannelCostRatio`。

因为折进的是 `GroupRatio`，所有计费路径自动继承，包括：

- 按量计费（`modelRatio × groupRatio`）
- 按次计费（`modelPrice × groupRatio`）
- `tiered_expr` 表达式计费（`quotaBeforeGroup × groupRatio`）
- 写入日志的 quota，与实际扣费一致

## 数据落点

存在渠道的 `setting` JSON 里，不新增数据库列，`omitempty` 保证旧渠道与旧前端不受影响：

```json
{
  "proxy": "",
  "channel_cost_ratio": 0.85
}
```

`<= 0` 视为未配置，等价于 1。

## 怎么用

### 设置

```bash
curl -X PUT "http://127.0.0.1:3000/api/channel/" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id": 29, "setting": {"channel_cost_ratio": 0.85}}'
```

读取当前值：

```bash
curl "http://127.0.0.1:3000/api/channel/29" -H "Authorization: Bearer $ADMIN_TOKEN"
```

### 典型场景：一个分组，多条上游

| 渠道 | 上游 | `channel_cost_ratio` |
|---|---|---|
| ZeroCat-A | 主力线 | 1.0（或留空） |
| ZeroCat-B | 便宜线 | 0.6 |
| 官方线 | 正价池 | 3.0 |

三条渠道都在 `【Claude】Max纯血分组` 里，`priority` 按成本从低到高排列。用户看到的价格只有一个，后台结算按实际命中的渠道走。

### 与定价守护进程配合

外部进程按周期拉取各上游 `/api/pricing`，算出每条渠道应设的倍率，再通过上面的接口写入。因为倍率是随请求读取的，写入后下一次请求立即生效，不需要重启。

## 注意事项

1. **倍率会同时缩放输入与输出价格**，比例关系由 `CompletionRatio` 保持。
2. **后台表单保存不会清空**该字段（补丁做了保留），但后台界面目前不显示它，管理入口只有 API。
3. **日志里的 group_ratio 是折算后的值**，等于 `分组倍率 × 渠道倍率`。排查时看到 0.51 这种数字，是 0.6 的分组倍率乘了 0.85 的渠道倍率，不是配置错误。
4. 上游 `GroupSpecialRatio`（用户组特殊倍率）仍然优先，渠道倍率在其之上再乘一次。

## 构建

仓库不包含 `web/dist`，直接 `go build` 会因 embed 失败。用官方 Dockerfile：

```bash
docker build -t new-api:channel-cost-ratio .
```

或先构建前端再编译：

```bash
cd web && bun install && bun run build && cd ..
go build -o new-api .
```

## 升级

本补丁只碰 4 个文件，且都是增量改动（新增字段与一个 if 块）。合并上游新版本时：

```bash
git fetch upstream
git rebase upstream/master
```

冲突大概率只出现在 `HandleGroupRatio` 函数的尾部，保留补丁那一段即可。
