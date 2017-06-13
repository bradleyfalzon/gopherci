#!/bin/bash -eux

# Expects only previous commit

git checkout $1

cp .git/config config
rm -rf .git
git init
mv config .git/

cat > foo.go <<EOF
package foo
func Foo() {}  // expect golint exported without comment
EOF

git add .
git commit -m "commit"

git push -f -u origin $1
