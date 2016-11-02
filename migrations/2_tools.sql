-- +migrate Up
CREATE TABLE tools (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    name varchar(64) NOT NULL,
    url varchar(128) NOT NULL,
    path varchar(64) NOT NULL,
    args varchar(128) NOT NULL,
    `regexp` varchar(128) NOT NULL,
    PRIMARY KEY (id)
);

INSERT INTO tools (name, url, path, args, `regexp`) VALUES
    ("go vet", "https://golang.org/cmd/vet/", "go", "vet ./...", ""),
    ("golint", "https://github.com/golang/lint", "golint", "./...", ""),
    ("apicompat", "https://github.com/bradleyfalzon/apicompat", "apicompat", "-before %BASE_BRANCH% ./...", ".*?:(.*?\.go):([0-9]+):()(.*)");

-- +migrate Down
DROP TABLE tools;
