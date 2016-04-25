#!/bin/bash
set -e
set -x

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $THIS_SCRIPT_DIR
npm install

cd $BITRISE_SOURCE_DIR


git checkout ${TAG_DEST}~1
previousTag=$(git describe --tags --abbrev=0)
git checkout ${TAG_DEST}

if [ -n "$CHANGE_FILE" ] ; then
	touch $CHANGE_FILE
$THIS_SCRIPT_DIR/changelog.js $TAG_DEST "${CHANGE_FILE}" $previousTag
git add $CHANGE_FILE
git commit -m "chore(${TAG_DEST}):update changes"
git push origin HEAD
fi

envman add --key CHANGELOG --value "#{$($THIS_SCRIPT_DIR/changelog.js $TAG_DEST '' $previousTag)}"

