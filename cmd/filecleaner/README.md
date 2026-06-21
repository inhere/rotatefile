# filecleaner

`filecleaner` 是基于 [`rotatefile`](../../) 的 `FilesClear` 实现的小工具,用于按 pattern 清理过期/超量的日志或备份文件。配置使用 JSON 文件,支持一次性运行与守护进程模式。

## 安装

```bash
go install github.com/gookit/rotatefile/cmd/filecleaner@latest
# 或在本目录下
go build -o filecleaner .
# 或在仓库根目录用 Makefile 构建(注入版本信息,产物输出到仓库根)
make build
```

## 使用

```bash
# 一次性清理
filecleaner -c filecleaner.json

# 预演:只打印将删除的文件,不实际删除
filecleaner --dry-run -c filecleaner.json

# 守护模式:按 check_interval 周期清理,Ctrl+C 退出
filecleaner --daemon -c filecleaner.json
```

选项:

| 选项 | 短选项 | 说明 | 默认 |
|------|--------|------|------|
| `--config` | `-c` | JSON 配置文件路径 | `filecleaner.json` |
| `--daemon` | `-d` | 守护模式,周期清理 | `false` |
| `--dry-run` | | 只打印将删除的文件,不删除 | `false` |

## 配置

配置文件顶层是一个 `jobs` 数组,每个 job 是一组独立的清理策略,可指定多个 pattern。

```json
{
  "jobs": [
    {
      "patterns": ["/var/log/app/*.log.*"],
      "backup_num": 20,
      "backup_time": 168,
      "time_unit": "1h",
      "compress": true
    },
    {
      "patterns": ["/var/log/svc"],
      "recursive": true,
      "remove_empty_dir": true,
      "backup_time": 7,
      "time_unit": "24h",
      "ignore_error": true
    }
  ]
}
```

字段说明(单个 job):

| 字段 | 类型 | 说明 |
|------|------|------|
| `patterns` | `[]string` | 匹配 pattern;可为文件 glob 或目录,如 `/tmp/err.log.*`、`/path/to/dir` |
| `backup_num` | `uint` | 按数量保留最新 N 个;`0` 表示不限制 |
| `backup_time` | `uint` | 按时间保留(单位为 `time_unit`);`0` 表示不限制 |
| `time_unit` | `string` | `backup_time` 的单位,时长字符串,如 `1h`、`24h`。默认 `1h` |
| `check_interval` | `string` | 守护模式的检查周期,如 `60s`。默认 `60s` |
| `compress` | `bool` | 暂留字段(由 `rotatefile.Writer` 使用),清理工具不主动压缩 |
| `recursive` | `bool` | 匹配到目录时递归处理子目录文件 |
| `remove_empty_dir` | `bool` | 清理后删除变空的子目录,仅在 `recursive` 下生效 |
| `ignore_error` | `bool` | 单个文件删除出错时忽略并继续 |

> 注意:`backup_num` 与 `backup_time` 不能同时为 `0`(否则该 job 无任何保留策略,会报错)。
> 当 `recursive=true` 时,`backup_num`/`backup_time` 作用于该 pattern 收集到的全部文件(含嵌套)这一整体池。

参考完整示例:[`filecleaner.example.json`](./filecleaner.example.json)。
