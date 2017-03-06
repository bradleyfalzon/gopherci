#!/bin/bash -eux

git checkout -b issue-comments-ignore-generated

cat > generated.go <<EOF
// automatically generated - DO NOT EDIT
package foo
func Foo() {}  // expect golint exported without comment
EOF

mkdir testdata
cat > testdata/foo.go <<EOF
package foo
func Foo() {}  // expect golint exported without comment
EOF

mkdir vendor
cat > vendor/foo.go <<EOF
package foo
func Foo() {}  // expect golint exported without comment
EOF

git add .
git commit -m "commit"

git push -f -u origin HEAD
