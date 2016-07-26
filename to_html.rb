#!/usr/bin/env ruby

require 'optparse'
require 'redcarpet'

# Input validation
options = {
  md: nil
}

parser = OptionParser.new do|opts|
  opts.banner = 'Usage: step.rb [options]'
  opts.on('-m', '--md  text', 'markdown text') { |b| options[:md] = b unless b.to_s == '' }
  opts.on('-h', '--help', 'Displays Help') do
    exit
  end
end
parser.parse!

markdown = Redcarpet::Markdown.new(Redcarpet::Render::HTML)
puts markdown.render options[:md]