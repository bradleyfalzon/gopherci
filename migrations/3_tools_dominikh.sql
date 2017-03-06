-- +migrate Up
INSERT INTO tools (name, url, path, args, `regexp`) VALUES
    ("gosimple", "https://github.com/dominikh/go-tools", "gosimple", "./...", ""),
    ("staticcheck", "https://github.com/dominikh/go-tools", "staticcheck", "./...", ""),
    ("unused", "https://github.com/dominikh/go-tools", "unused", "./...", "");

-- +migrate Down
-- +migrate StatementBegin
-- +migrate StatementEnd
