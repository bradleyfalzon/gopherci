-- +migrate Up
CREATE TABLE gh_installations (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    installation_id INT UNSIGNED NOT NULL,
    account_id INT UNSIGNED NOT NULL,
    sender_id INT UNSIGNED NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enabled_at TIMESTAMP NULL DEFAULT NULL,
    updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    INDEX account_id (account_id),
    UNIQUE installation_id (installation_id)
);

-- +migrate Down
DROP TABLE gh_installations;
