-- +migrate Up
INSERT INTO tools (name, url, path, args, `regexp`) VALUES
    ("unconvert", "https://github.com/mdempsky/unconvert", "unconvert", "./...", "");

-- +migrate Down
-- +migrate StatementBegin
-- +migrate StatementEnd
