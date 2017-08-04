-- +migrate Up
CREATE TABLE outputs (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    analysis_id INT UNSIGNED NOT NULL,
    arguments VARCHAR(2048) NOT NULL,
    duration TIME(3) NOT NULL,
    output TEXT NOT NULL,
    PRIMARY KEY (id),
    KEY (analysis_id),
    FOREIGN KEY (analysis_id) REFERENCES analysis(id) ON DELETE CASCADE
);

-- +migrate Down
DROP TABLE outputs;
