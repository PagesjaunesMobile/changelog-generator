#!/bin/bash
set -e
set -x

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $THIS_SCRIPT_DIR
npm install

cd $BITRISE_SOURCE_DIR

ALREADY=$(grep "^# ${TAG_DEST}" $CHANGE_FILE) 
git checkout ${TAG_DEST}~1
previousTag=$(git describe --tags --abbrev=0)
git checkout ${TAG_DEST}
git config --global user.email $IC_COMMITER_MAIL
git config --global user.name $IC_COMMITER_NAME

if [ -n "$CHANGE_FILE" ] ; then
	touch $CHANGE_FILE
	 # or ALREADY = tag <> HEAD
	if [ -z "$ALREADY" ]; then 
		$THIS_SCRIPT_DIR/changelog.js $TAG_DEST "${CHANGE_FILE}" $previousTag
		GIT_ASKPASS=echo 
		GIT_SSH="${THIS_SCRIPT_DIR}/ssh_no_prompt.sh"
		git add $CHANGE_FILE
		git commit -m "chore(${TAG_DEST}):update changes"
		git push origin HEAD:$BITRISE_GIT_BRANCH
	fi
fi

envman add --key CHANGELOG --value "$($THIS_SCRIPT_DIR/changelog.js $TAG_DEST '' $previousTag)"

