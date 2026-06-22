-- 为 server_deploy_logs 增加二级索引，覆盖按发布单点查与 ClaimNextDeploy 的互斥自连接。
CREATE INDEX idx_sdl_deploy ON server_deploy_logs(deploy_record_id);
CREATE INDEX idx_sdl_server ON server_deploy_logs(server_id);
