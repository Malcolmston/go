# Parity driver for CRuby: requires the REAL gem installed alongside it and
# answers parity cases over newline-delimited JSON.
#
# Each request is {id, src, input}. `src` is evaluated as a lambda body with
# `input` in scope; its value is the upstream answer and anything it raises is
# upstream's error behavior.
require 'json'
require 'base64'
require 'date'

if ENV['GEM_HOME']
  Gem.paths = { 'GEM_HOME' => ENV['GEM_HOME'], 'GEM_PATH' => ENV['GEM_HOME'] }
  Gem.clear_paths
end

# encode maps Ruby values onto the canonical JSON the Go side also encodes to.
def encode(v, seen = [])
  case v
  when nil, true, false, Integer, String then v
  when Float
    return 'NaN' if v.nan?
    return v.positive? ? 'Infinity' : '-Infinity' if v.infinite?

    v
  when Symbol then v.to_s
  when Time, Date, DateTime then v.iso8601
  when Proc, Method then { '$fn' => 'anonymous' }
  when Exception then { '$error' => v.message }
  else
    return { '$cycle' => true } if seen.include?(v.object_id)

    seen = seen + [v.object_id]
    case v
    when Array then v.map { |x| encode(x, seen) }
    when Set then v.to_a.map { |x| encode(x, seen) }
    when Hash then v.to_h { |k, x| [k.to_s, encode(x, seen)] }
    else
      return encode(v.to_h, seen) if v.respond_to?(:to_h)
      return encode(v.as_json, seen) if v.respond_to?(:as_json)

      v.to_s
    end
  end
end

$stdin.each_line do |line|
  next if line.strip.empty?

  begin
    req = JSON.parse(line)
  rescue StandardError => e
    $stdout.puts(JSON.generate({ 'id' => '', 'ok' => false, 'error' => "bad request: #{e.message}" }))
    $stdout.flush
    next
  end

  reply = begin
    fn = eval("lambda { |input| #{req['src']} }", binding, '(parity-case)', 1) # rubocop:disable Security/Eval
    { 'id' => req['id'], 'ok' => true, 'value' => encode(fn.call(req['input'])) }
  rescue Exception => e # rubocop:disable Lint/RescueException - upstream raising is data
    { 'id' => req['id'], 'ok' => false, 'error' => "#{e.class}: #{e.message}" }
  end
  $stdout.puts(JSON.generate(reply))
  $stdout.flush
end
