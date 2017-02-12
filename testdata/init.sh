#!/bin/bash -eux

CLONE_URL=$1

git init > /dev/null
git config --local user.name "testdata"
git config --local user.email "testdata@example.com"
touch readme
git add .
git commit -m "Initial commit" > /dev/null
git remote add origin $1
git push -f -u origin master
