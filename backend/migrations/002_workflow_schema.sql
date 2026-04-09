-- 002_workflow_schema.sql
-- 工作流编排系统 schema

CREATE TABLE IF NOT EXISTS workflows (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    owner_type      ENUM('system','user') NOT NULL DEFAULT 'user',
    owner_id        BIGINT UNSIGNED NOT NULL,
    source_id       BIGINT UNSIGNED DEFAULT NULL COMMENT '克隆来源工作流ID',
    is_published    TINYINT(1) NOT NULL DEFAULT 0,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_owner (owner_type, owner_id),
    INDEX idx_published (is_published)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS workflow_nodes (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    workflow_id     BIGINT UNSIGNED NOT NULL,
    node_type       VARCHAR(50) NOT NULL,
    position        INT NOT NULL DEFAULT 0,
    config          JSON,
    enabled         TINYINT(1) NOT NULL DEFAULT 1,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_workflow (workflow_id),
    CONSTRAINT fk_wfnode_workflow FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS workflow_executions (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    workflow_id     BIGINT UNSIGNED NOT NULL,
    trigger_type    VARCHAR(50) NOT NULL COMMENT 'batch_task|realtime|manual',
    trigger_id      VARCHAR(128) DEFAULT NULL COMMENT '触发来源ID',
    input_text      LONGTEXT,
    final_text      LONGTEXT,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message   TEXT,
    started_at      DATETIME(3) DEFAULT NULL,
    completed_at    DATETIME(3) DEFAULT NULL,
    created_at      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    INDEX idx_workflow (workflow_id),
    INDEX idx_trigger (trigger_type, trigger_id),
    INDEX idx_status (status),
    CONSTRAINT fk_wfexec_workflow FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS workflow_node_results (
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    execution_id    BIGINT UNSIGNED NOT NULL,
    node_id         BIGINT UNSIGNED NOT NULL,
    node_type       VARCHAR(50) NOT NULL,
    position        INT NOT NULL DEFAULT 0,
    input_text      LONGTEXT,
    output_text     LONGTEXT,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    detail          JSON,
    duration_ms     INT NOT NULL DEFAULT 0,
    executed_at     DATETIME(3) DEFAULT NULL,
    INDEX idx_execution (execution_id),
    CONSTRAINT fk_wfnr_execution FOREIGN KEY (execution_id) REFERENCES workflow_executions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 为转写任务增加工作流绑定
ALTER TABLE transcription_tasks ADD COLUMN workflow_id BIGINT UNSIGNED DEFAULT NULL AFTER dict_id;
ALTER TABLE transcription_tasks ADD INDEX idx_workflow_id (workflow_id);
