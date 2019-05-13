#!/bin/sh
rm -rf public
rm -rf amethyst

echo "Creating folders..."
mkdir -p public/book/
mkdir -p amethyst/

echo "Installing dependencies..."
cargo install-update --version || cargo install cargo-update
mdbook --version || cargo install mdbook
cargo install-update -a

echo "Cloning amethyst..."
git clone https://github.com/amethyst/amethyst --branch master amethyst/master
cd amethyst/master

echo "Compiling master branch book"
mdbook build book

cd ../../

echo "Moving book to /public/"
mkdir -p public/book/master/
mv -f amethyst/master/book/book/* public/book/master/

cd amethyst/master
LATEST_TAG=$(git describe --abbrev=0 --tags)
echo "Checking out $LATEST_TAG"
git checkout -q $LATEST_TAG

echo "Compiling $LATEST_TAG book"
mdbook build book

cd ../../

echo "Moving book to /public/"
mkdir -p public/book/stable/
mv -f amethyst/master/book/book/* public/book/stable

cd amethyst/master
for tag in $(git tag)
do
    echo "Checking out $tag"
    git checkout -q $tag

    echo "Compiling $tag book"
    mdbook build book

    cd ../../

    mkdir -p build/book/$tag/
    mv -f amethyst/master/book/book/* build/book/$tag/

    cd amethyst/master
done
cd ../../