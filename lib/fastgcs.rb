# frozen_string_literal: true

require('net/http')
require('json')
require('time')
require('tmpdir')
require('fileutils')

class FastGCS
  CREDENTIALS_DB = File.expand_path("~/.config/gcloud/credentials.db")
  ACCESS_TOKENS_DB = File.expand_path("~/.config/gcloud/access_tokens.db")
  CREDENTIAL_CACHE = File.expand_path("~/.config/gcloud/com.shopify.fastgcs.json")
  CACHE = File.expand_path("~/.cache/fastgcs")

  autoload(:VERSION, 'fastgcs/version')

  UsageError = Class.new(StandardError)
  def self.run_cli(*argv)
    case argv.shift
    when 'cat'
      raise(UsageError) unless argv.size == 1
      IO.copy_stream(new.open(argv.first), STDOUT)
    when 'cp'
      raise(UsageError) unless argv.size == 2
      new.cp(argv.first, argv[1])
    else
      raise(UsageError)
    end
  rescue UsageError
    abort(<<~EOF)
      usage:
        #{$PROGRAM_NAME} cp gs://url ./path
        #{$PROGRAM_NAME} cat gs://url
    EOF
  end

  def initialize
    FileUtils.mkdir_p(CACHE)
    load_credentials
    ensure_current_token
  end

  def open(url)
    path = update(url)
    File.open(path, 'r')
  end

  def cp(url, to)
    path = update(url)
    FileUtils.cp(path, to)
  end

  def read(url)
    path = update(url)
    File.read(path)
  end

  private

  def cache_path(url)
    bucket, object = parse_gs_url(url)
    File.join(CACHE, "#{bucket}--#{object.tr('/', '-')}")
  end

  def update(url)
    path = cache_path(url)
    Net::HTTP.start('storage.googleapis.com', 443, use_ssl: true) do |http|
      update_http(http, path, url)
    end
    path
  end

  def update_http(http, path, url)
    etag = begin
      File.read(etag_path(path))
    rescue Errno::ENOENT
      nil
    end
    Dir.mktmpdir do |dir|
      file = File.join(dir, 'file')
      File.open(file, File::WRONLY|File::CREAT, 0644) do |f|
        new_etag = get(http, url, f, etag: etag)
        if new_etag
          FileUtils.mv(file, path)
          File.write(etag_path(path), new_etag)
          info("updated #{url}")
        else
          info("#{url} already current")
        end
      end
    end
    nil
  end

  def info(msg)
    STDERR.puts("[FastGCS] #{msg}") if ENV['FASTGCS_LOG']
  end

  def etag_path(path)
    File.join(File.dirname(path), ".#{File.basename(path)}.etag")
  end

  def get(http, url, io, etag: nil)
    ensure_current_token

    bucket, object = parse_gs_url(url)
    headers = {
      'Authorization' => "Bearer #{@token}",
      'Accept-Encoding' => 'gzip;q=1.0',
    }
    headers['If-None-Match'] = etag if etag
    req = Net::HTTP::Get.new("/storage/v1/b/#{bucket}/o/#{object}?alt=media", headers)
    http.request(req) do |resp|
      case resp.code.to_i
      when 200
        resp.read_body { |chunk| io.write(chunk) }
        return(resp['ETag'])
      when 304
        return nil
      else
        raise("HTTP error #{resp.code}")
      end
    end
  end

  def load_credentials
    credsdb = File.read(CREDENTIALS_DB, encoding: Encoding::BINARY)
    # printable characters and newline: valid json will only contain these,
    # and the valid json document will immediately follow the email address in
    # sqlite3 format and this schema.
    creds_json = credsdb.scan(/@shopify\.com([\x0A\x20-\x7F]+)/).flatten.first
    creds = JSON.load(creds_json)
    @client_id = creds.fetch('client_id')
    @client_secret = creds.fetch('client_secret')
    @refresh_token = creds.fetch('refresh_token')
    nil
  end

  def parse_gs_url(url)
    unless url.match(%r{^gs://([^/]+)/(.*)$})
      raise(ArgumentError, "Invalid gs:// URL")
    end
    bucket = Regexp.last_match(1)
    object = Regexp.last_match(2)
    [bucket, object]
  end

  def ensure_current_token
    return if @token && @expiry > Time.now

    found = false
    found ||= try_access_token_from_cache
    found ||= try_access_token_from_gcloud_db
    found ||= refresh_access_token

    update_cache
  end

  def try_access_token_from_cache
    data = JSON.parse(File.read(CREDENTIAL_CACHE))
    token = data.fetch('token')
    expiry = Time.parse(data.fetch('expiry'))
    if expiry > Time.now
      @expiry = expiry
      @token = token
      return(true)
    end
    false
  rescue Errno::ENOENT
    return(false)
  end

  def update_cache
    File.write(CREDENTIAL_CACHE, { token: @token, expiry: @expiry }.to_json)
    File.open(CREDENTIAL_CACHE, File::WRONLY|File::CREAT, 0600) do |f|
      f.write({ token: @token, expiry: @expiry }.to_json)
    end
  end

  def try_access_token_from_gcloud_db
    db = File.read(ACCESS_TOKENS_DB, encoding: Encoding::BINARY)
    # printable characters and newline: valid json will only contain these,
    # and the valid json document will immediately follow the email address in
    # sqlite3 format and this schema.
    data = db.scan(/@shopify\.com(.{171})(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)/).flatten
    unless data.size == 2
      return(false) # parse fail? no data?
    end
    expiry = Time.parse("#{data[1]} UTC")
    if expiry > Time.now
      @expiry = expiry
      @token = data[0]
      return(true)
    end
    false
  end

  def refresh_access_token
    Net::HTTP.start('oauth2.googleapis.com', 443, use_ssl: true) do |http|
      req = Net::HTTP::Post.new('/token', {
        'User-Agent' => 'curl',
        'Content-Type' => 'application/json',
      })
      req.body = {
        client_id: @client_id,
        client_secret: @client_secret,
        refresh_token: @refresh_token,
        grant_type: 'refresh_token',
      }.to_json
      resp = http.request(req)
      raise("HTTP error #{resp.code}") unless resp.code.to_i == 200
      obj = JSON.parse(resp.body)
      @token = obj.fetch('access_token')
      @expiry = Time.now + obj.fetch('expires_in')
    end
    true
  end
end
