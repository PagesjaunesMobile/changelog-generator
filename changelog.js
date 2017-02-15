#!/usr/bin/env node

// get previous Tag
// tag new version
//change new previous
//tag
// commit chore(version):update changelog
//push


'use strict';

if (!Array.prototype.last){
  Array.prototype.last = function(){
    return this[this.length - 1];
  };
};

var fs = require('fs');
var util = require('util');
var child = require('child_process');

var q = require('qq');

var lite = false;
var GIT_LOG_CMD = 'git log --invert-grep --grep="%s" -E --format=%s %s..%s';
var GIT_TAG_CMD = 'git describe --tags --abbrev=0';
var GIT_TAG_DATE_CMD = 'git log -1 --format=%ai %s';
var HEADER_TPL = "\n<a name='%s'></a>\n# %s (%s)\n\n";
var HEADLESS_TPL = "";
var LINK_ISSUE_LITE = '[#%s]';
var LINK_FEATURE_LITE = "[%s]";
var LINK_COMMIT_LITE = '[%s]';
var LINK_ISSUE = '[#%s](' + process.env.ISSUE_TRACKER + '/%s)';
var LINK_FEATURE = "[%s](https://wiki.services.local/dosearchsite.action?spaceSearch=false&queryString='%s')";
  var LINK_COMMIT = '([%s](' + process.env.GIT_COMMIT_LINK + '/%s))';

  var EMPTY_COMPONENT = '$$';


  var warn = function() {
    console.error('WARNING:', util.format.apply(null, arguments));
  };


  var parseRawCommit = function(raw) {
    if (!raw) return null;

    var lines = raw.split('\n');
    var msg = {}, match;

    msg.hash = lines.shift();
    msg.subject = lines.shift();
    msg.closes = [];
    msg.breaks = [];

    lines.forEach(function(line) {
      match = line.match(/(?:Closes|Fixes|Features|closes|fixes|features)\s#?([A-Z0-9_\-]+)/);
      if (match) msg.closes.push(match[1]);
    });

    match = raw.match(/BREAKING CHANGE:([\s\S]*)/);

    if (match) {
      console.error("BREAK >%s<", match[1]);
      msg.breaking = match[1];
    }

    var featType= ["pods", "pod", "config", "conf", "feat", "refactor", "codereview"];
    msg.body = lines.join('\n');
    if(msg.subject.indexOf('Merge ')<0){
      match = msg.subject.match(/^(.*)\((.*)\)\s*\:\s*(.*)$/);
      if (!match || !match[1] || !match[3]) {
        match = msg.subject.match(/^\[([^ \]]*)\]\s*\[([^\]]*)\]\s*(.*)$/);
      }

      if (!match || !match[1] || !match[3]) {
        warn('Incorrect message: %s %s', msg.hash, msg.subject);
        msg.type='bof';
        msg.component='?';
        msg.subject= msg.subject+ " par *" +  authorCommit(msg.body) +"*";
        // console.log("\nHERE >>>>> %s \n>>>> %s\n ", msg.subject, msg.body);
      } else {
        msg.type = match[1].toLowerCase();
        var prefixe = "";
        if (featType.indexOf(msg.type)>-1) {
          prefixe = "[" + msg.type + "] - "
          msg.type="feat"
        }
        msg.component = match[2];
        msg.subject = prefixe + match[3];
      }
      return msg;
    } else
    return null;
  };
  var authorCommit = function(commit) {
    return commit.split('\n').last();
  };

  var linkToIssue = function(issue) {
    if (issue.match(/^[A-Z]+-[0-9]+$/)) {
      if (lite == true){
        return util.format(LINK_ISSUE_LITE, issue);
      } else
      return util.format(LINK_ISSUE, issue, issue);
    }
    if (lite == true){
      return util.format(LINK_FEATURE_LITE, issue);
    } else
    return util.format(LINK_FEATURE, issue, issue);
  };


  var linkToCommit = function(hash) {
    if (lite == true){
      return "";
    } else
    return util.format(LINK_COMMIT, hash.substr(0, 8), hash);
  };


  var currentDate = function(date) {
    var now;
    if (!date)
      now = new Date();
    else
      now = new Date(Date.parse(date));

    var pad = function(i) {
      return ('0' + i).substr(-2);
    };

    return util.format('%d-%s-%s', now.getFullYear(), pad(now.getMonth() + 1), pad(now.getDate()));
  };


  var printSection = function(stream, title, section, printCommitLinks) {
    
    printCommitLinks = printCommitLinks === undefined ? true : printCommitLinks;
    var components = Object.getOwnPropertyNames(section).sort();

    if (!components.length || components[0]==EMPTY_COMPONENT ){
      return;
    };

    stream.write(util.format('\n\n## %s', title));
    components.forEach(function(name) {
      var prefix = "\n  -";
      var nested = section[name].length > 1;

      if (name !== EMPTY_COMPONENT) {
        if (nested) {

          stream.write(util.format("\n  - **%s:**\n", name));
          prefix = "\n    -";
        } else {
          prefix = util.format("\n  - **%s:**", name);
        }
      }

      var doublon="";
      section[name].forEach(function(commit) {

        if (printCommitLinks) {
          if (doublon == commit.subject)
          {
            stream.write(linkToCommit(commit.hash));
          } else {
            stream.write(util.format("%s %s\n  %s", prefix, commit.subject, linkToCommit(commit.hash)));
          }
          if (commit.closes.length) {
            stream.write(",\n   " + commit.closes.map(linkToIssue).join(', '));
          }
        } else {
          if (doublon != commit.subject){
            stream.write(util.format("%s %s\n", prefix, commit.subject));
          }
        }
        doublon=commit.subject;
      });
    });
  };


  var readGitLog = function(grep, from, to) {
    var deferred = q.defer();

    // TODO(vojta): if it's slow, use spawn and stream it instead
    console.error(util.format(GIT_LOG_CMD, grep, '%H%n%B%n%an%n==END==', from, to));
    //console.log(child.exec("pwd; cat .git/config"));
    child.exec(util.format(GIT_LOG_CMD, grep, '%H%n%B%n%an%n==END==', from, to), function(code, stdout, stderr) {
      var commits = [];
      stdout.split('\n==END==\n').forEach(function(rawCommit) {
        var commit = parseRawCommit(rawCommit);
        if (commit) commits.push(commit);
      });

      deferred.resolve(commits);
    });

    return deferred.promise;
  };


  var writeChangelog = function(data, stream, commits, version, date) {
    var sections = {
      fix: {},
      feat: {},
      perf: {},
      bof: {},
      breaks: {}
    };

    sections.breaks[EMPTY_COMPONENT] = [];

    commits.forEach(function(commit) {
      var section = sections[commit.type];
      var component = commit.component || EMPTY_COMPONENT;

      if (section) {        
        section[component] = section[component] || [];
        section[component].push(commit);
      }

      if (commit.breaking) {
        sections.breaks[component] = sections.breaks[component] || [];
        sections.breaks[component].push({
          subject: util.format("due to %s,\n %s", linkToCommit(commit.hash), commit.breaking),
          hash: commit.hash,
          closes: []
        });
      }
    });

    if (version == "HEAD") {
      stream.write(util.format(HEADLESS_TPL, date));
    } else {
      stream.write(util.format(HEADER_TPL, version, version, currentDate(date)));
    }
  
    printSection(stream, 'Bug Fixes', sections.fix);
    printSection(stream, 'Features', sections.feat);
    printSection(stream, 'Performance Improvements', sections.perf);
    printSection(stream, 'Breaking Changes', sections.breaks, false);
    printSection(stream, 'non conforme', sections.bof);
    console.error("End ", data.length);
    stream.write("\n\n" + data);
  };


  var getPreviousTag = function() {
    var deferred = q.defer();
    child.exec(GIT_TAG_CMD, function(code, stdout, stderr) {
      if (code) deferred.reject('Cannot get the previous tag.');
      else deferred.resolve(stdout.replace('\n', ''));
    });
    return deferred.promise;
  };


  var generate = function(data, file, to, from) {
    var stream;


    stream = file ? fs.createWriteStream(file, { flags: 'w'}):process.stdout;

    var tagDate= child.execSync(util.format(GIT_TAG_DATE_CMD, to), function(error, stdout, stderr) {
      // command output is in stdout
      return stdout;
    });

    if(from==null){
      getPreviousTag().then(function(tag) {
        console.error('Reading git log since', tag);
        readGitLog('^Merge', tag, "HEAD").then(function(commits) {
          console.error('Parsed', commits.length, 'commits');
          console.error('Generating changelog to', file || 'stdout', '(', to, ')');
          //  console.error('>>>>>>',commits[0],'<<<<<')
          writeChangelog(data, stream, commits, to, tagDate);
        });
      });
    } else {
      console.error('Reading git log between %s and %s (%s)', from, to, tagDate.toString());
      readGitLog('^Merge', from, to).then(function(commits) {
        if (to=="HEAD"){
          commits[0].subject = commits[0].subject + " *Last*";
          tagDate = ""; // commits[0].subject + commits[0].subject+ commits[0].subject + "_ (" + authorCommit(commits[0].body) + ")"
        }
        console.error('Parsed', commits.length, 'commits');
        console.error('Generating changelog to', file || 'stdout', '(', to, ')');
        writeChangelog(data, stream, commits, to, tagDate);
      });
    };
    //  stream.write(data);
  }


  // publish for testing
  exports.parseRawCommit = parseRawCommit;
  exports.printSection = printSection;
  // hacky start if not run by jasmine :-D
  //console.error(child);
  if (process.argv.length > 5 && process.argv[5].indexOf("lite")>0){
  
    lite=true;
  }

  var file = process.argv[3];
console.error(process.argv[4])
  var chunk='';
  if (file){
    var streamOld = fs.createReadStream(file, {encoding: 'utf8'});

    streamOld.on('readable', function() {
      var buf;
      while ((buf = streamOld.read()) !== null) {
        chunk = chunk+buf;
      }
    });

    streamOld.on( 'end' ,  function ()  {
      console.error( 'Read completed successfully.' );
      generate(chunk, file, process.argv[2], process.argv[4]);
    });

  } else
  generate('', null, process.argv[2], process.argv[4]);

