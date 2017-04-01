-- +migrate Up

-- commit_from and commit_to is before/after or from/to for GitHub or GitLab respectively
ALTER TABLE analysis ADD COLUMN commit_from VARCHAR(128) NULL DEFAULT NULL AFTER repository_id;
ALTER TABLE analysis ADD COLUMN commit_to VARCHAR(128) NULL DEFAULT NULL AFTER commit_from;

-- request number is pull request number or merge request number for GitHub or GitLan respectively
ALTER TABLE analysis ADD COLUMN request_number INT UNSIGNED NULL DEFAULT NULL AFTER commit_to;

-- +migrate Down
ALTER TABLE analysis DROP COLUMN commit_from, DROP COLUMN commit_to, DROP COLUMN request_number;


