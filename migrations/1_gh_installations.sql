-- +migrate Up
CREATE TABLE gh_installations (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    installation_id INT UNSIGNED NOT NULL,
    account_id INT UNSIGNED NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE installation_account_id (account_id, installation_id)
);

-- +migrate Down
DROP TABLE gh_installations;
