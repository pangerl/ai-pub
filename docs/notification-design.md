# 通知设计

## 1. 目标

第一版通知只实现企业微信机器人 webhook，用于关键发布事件提醒。

默认通知事件：

- 生产待确认。
- 发布失败。
- 回滚申请。

通知失败不能阻塞主发布流程。

## 2. 非目标

- 不做企业微信应用消息。
- 不做复杂通知订阅中心。
- 不做用户级细粒度订阅。
- 不把通知逻辑散落到发布状态判断里。

## 3. 模型

### 3.1 NotificationConfig

字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `channel` | 固定为 `wecom_robot` |
| `name` | 配置名称 |
| `webhook_url_enc` | 加密 webhook |
| `enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间 |

### 3.2 NotificationDelivery

字段：

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `config_id` | 通知配置 |
| `event_type` | 事件类型 |
| `release_request_id` | 发布单，可空；测试发送不关联发布单 |
| `deploy_record_id` | 发布记录，可空 |
| `status` | `sent` / `failed` |
| `last_error` | 错误摘要 |
| `sent_at` | 发送时间 |
| `created_at` / `updated_at` | 时间 |

## 4. 通知事件

| 事件 | 触发条件 | 默认发送 |
|------|----------|----------|
| `prod_pending_confirm` | 生产发布单等待管理员确认 | 是 |
| `deploy_failed` | 发布记录最终失败或 partial | 是 |
| `rollback_requested` | 创建回滚发布单 | 是 |
| `deploy_success` | 发布成功 | 否，可后续配置 |

事件来源：

- 发布单创建后进入生产待管理员确认状态。
- 发布记录聚合终态。
- 回滚创建流程。

## 5. 发送流程

```text
业务事件发生
  -> NotificationService 读取配置
  -> 生成企业微信消息
  -> 调用机器人 webhook
  -> 创建 NotificationDelivery(sent/failed)
  -> 写 ReleaseEvent(notification_sent/notification_failed)
```

要求：

- 通知发送失败不回滚发布事务。
- 通知错误写入发送记录。
- 通知内容不得包含未脱敏凭据、token、私钥、完整 webhook。

## 6. 企业微信机器人 webhook

配置项：

- webhook URL。
- enabled。
- 名称。

测试发送：

- 通知配置页面提供测试发送。
- 测试发送写入 `NotificationDelivery`，但不关联发布单。

## 7. 消息模板

### 7.1 生产待确认

内容：

```text
【发布待确认】
服务：{service_name}
环境：{environment_name}
版本：{version}
申请人：{created_by}
发布单：{release_request_id}
请管理员进入发布中心确认。
```

### 7.2 发布失败

内容：

```text
【发布失败】
服务：{service_name}
环境：{environment_name}
版本：{version}
状态：{deploy_status}
失败服务器：{failed_count}
错误摘要：{error_summary}
发布单：{release_request_id}
```

`partial` 按失败通知。

### 7.3 回滚申请

内容：

```text
【回滚申请】
服务：{service_name}
环境：{environment_name}
回滚版本：{rollback_version}
原发布单：{original_release_request_id}
新发布单：{release_request_id}
```


## 9. 扩展点

通知渠道接口：

```text
Send(ctx, message) -> result
```

后续渠道：

- 邮件。
- 飞书。
- Slack。
- 通用 webhook。

约束：

- 新渠道复用通知事件和发送记录。
- 新渠道不直接判断发布状态。

## 10. 验证要求

- 配置企业微信机器人 webhook 后，测试发送成功。
- 生产待确认触发通知。
- 发布失败触发通知。
- 回滚申请触发通知。
- webhook 调用失败时主发布流程不失败。
- 通知日志和事件可查询。
