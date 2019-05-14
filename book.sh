#!/bin/sh
rm -rf public
rm -rf amethyst

echo "Creating folders..."
mkdir -p public/book/
mkdir -p amethyst/

echo "Updating tooling..."
cargo install-update -a

echo "Cloning amethyst..."
git clone https://github.com/amethyst/amethyst --branch master amethyst

echo "Compiling master branch book..."
pushd amethyst
mdbook build book
popd

echo "Moving master to public dir..."
mkdir -p public/master/
mv -f amethyst/book/book/* public/master/

echo "Compiling stable ($LATEST_TAG)..."
pushd amethyst
LATEST_TAG=$(git describe --abbrev=0 --tags)
git checkout -q $LATEST_TAG
mdbook build book
popd

echo "Moving stable to public dir..."
mkdir -p public/book/stable/
mv -f amethyst/book/book/* public/book/stable

pushd amethyst
for tag in $(git tag)
do
    echo "Compiling $tag..."
    git checkout -q $tag
    mdbook build book

    popd

    mkdir -p public/tags/$tag/
    mv -f amethyst/book/book/* public/tags/$tag/

    pushd amethyst
done
popd