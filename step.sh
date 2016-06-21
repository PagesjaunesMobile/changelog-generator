#!/bin/bash
set -e
set -x

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $THIS_SCRIPT_DIR
npm install

cd $BITRISE_SOURCE_DIR


if [ "$TAG_DEST" != "HEAD" ]; then
  git checkout ${TAG_DEST}~1
fi

previousTag=$(git describe --tags --abbrev=0)
git checkout ${TAG_DEST}
git config --global user.email $IC_COMMITER_MAIL
git config --global user.name $IC_COMMITER_NAME

if [ -e "$CHANGE_FILE" ] ; then
  ALREADY=$(grep "^# ${TAG_DEST}" $CHANGE_FILE) 
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
  #git log --invert-grep --grep="^Merge" -E --format=%H%n%s%n%b%n%an%n==END== ${previousTag}..${TAG_DEST}
envman add --key CHANGELOG --value "$($THIS_SCRIPT_DIR/changelog.js $TAG_DEST '' $previousTag)"

