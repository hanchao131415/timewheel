-- TimeWheel 数据库初始化脚本
-- 支持 MySQL 8.0+

-- 创建任务表
CREATE TABLE IF NOT EXISTS `tasks` (
    `id` VARCHAR(64) NOT NULL COMMENT '任务ID',
    `name` VARCHAR(255) NOT NULL COMMENT '任务名称',
    `description` VARCHAR(500) DEFAULT '' COMMENT '任务描述',
    `mode` TINYINT DEFAULT 0 COMMENT '执行模式(0:重复,1:单次,2:固定次数)',
    `interval_ms` BIGINT NOT NULL COMMENT '执行间隔(毫秒)',
    `times` INT DEFAULT 0 COMMENT '执行次数(固定次数模式)',
    `priority` TINYINT DEFAULT 1 COMMENT '优先级(0:高,1:普通,2:低)',
    `timeout_ms` BIGINT DEFAULT 0 COMMENT '超时时间(毫秒)',
    `severity` TINYINT DEFAULT 1 COMMENT '告警级别(0:严重,1:警告,2:信息)',
    `for_duration_ms` BIGINT DEFAULT 0 COMMENT '持续时间(毫秒)',
    `repeat_interval_ms` BIGINT DEFAULT 0 COMMENT '重复告警间隔(毫秒)',
    `labels` JSON DEFAULT NULL COMMENT '告警标签',
    `annotations` JSON DEFAULT NULL COMMENT '告警描述',
    `enabled` BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    `paused` BOOLEAN DEFAULT FALSE COMMENT '是否暂停',
    `enabled_at` DATETIME DEFAULT NULL COMMENT '启用时间',
    `paused_at` DATETIME DEFAULT NULL COMMENT '暂停时间',
    `executed_count` INT DEFAULT 0 COMMENT '已执行次数',
    `last_executed_at` DATETIME DEFAULT NULL COMMENT '最后执行时间',
    `last_result_value` DOUBLE DEFAULT 0 COMMENT '最后执行结果值',
    `last_is_firing` BOOLEAN DEFAULT FALSE COMMENT '最后是否触发告警',
    `alert_state` TINYINT DEFAULT 0 COMMENT '告警状态(0:待定,1:触发,2:解决)',
    `pending_since` DATETIME DEFAULT NULL COMMENT '进入Pending状态时间',
    `last_fired_at` DATETIME DEFAULT NULL COMMENT '最后触发告警时间',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME DEFAULT NULL COMMENT '删除时间',
    PRIMARY KEY (`id`),
    INDEX `idx_enabled` (`enabled`),
    INDEX `idx_paused` (`paused`),
    INDEX `idx_alert_state` (`alert_state`),
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务表';

-- 创建告警历史表
CREATE TABLE IF NOT EXISTS `alert_history` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
    `task_id` VARCHAR(64) NOT NULL COMMENT '任务ID',
    `task_name` VARCHAR(255) DEFAULT '' COMMENT '任务名称',
    `old_state` TINYINT DEFAULT NULL COMMENT '旧状态',
    `new_state` TINYINT DEFAULT NULL COMMENT '新状态',
    `value` DOUBLE DEFAULT NULL COMMENT '当前值',
    `threshold` DOUBLE DEFAULT NULL COMMENT '阈值',
    `is_firing` BOOLEAN DEFAULT FALSE COMMENT '是否触发',
    `notified` BOOLEAN DEFAULT FALSE COMMENT '是否已通知',
    `notified_at` DATETIME DEFAULT NULL COMMENT '通知时间',
    `severity` TINYINT DEFAULT NULL COMMENT '告警级别',
    `labels` JSON DEFAULT NULL COMMENT '标签',
    `annotations` JSON DEFAULT NULL COMMENT '描述',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    PRIMARY KEY (`id`),
    INDEX `idx_task_id` (`task_id`),
    INDEX `idx_created_at` (`created_at`),
    INDEX `idx_is_firing` (`is_firing`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='告警历史表';

-- 创建 Webhook 推送队列表
CREATE TABLE IF NOT EXISTS `webhook_queue` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键ID',
    `task_id` VARCHAR(64) DEFAULT NULL COMMENT '关联任务ID',
    `url` VARCHAR(500) NOT NULL COMMENT 'Webhook URL',
    `payload` JSON DEFAULT NULL COMMENT '推送载荷',
    `status` VARCHAR(20) DEFAULT 'pending' COMMENT '状态(pending/success/failed)',
    `attempts` INT DEFAULT 0 COMMENT '尝试次数',
    `last_attempt` DATETIME DEFAULT NULL COMMENT '最后尝试时间',
    `next_attempt` DATETIME DEFAULT NULL COMMENT '下次尝试时间',
    `error_msg` TEXT DEFAULT NULL COMMENT '错误信息',
    `created_at` DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    INDEX `idx_task_id` (`task_id`),
    INDEX `idx_status` (`status`),
    INDEX `idx_next_attempt` (`next_attempt`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Webhook推送队列表';
