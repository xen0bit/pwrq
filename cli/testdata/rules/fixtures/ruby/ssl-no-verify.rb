require 'net/http'
require 'openssl'

def fetch(uri)
  http = Net::HTTP.new(uri.host, uri.port)
  http.use_ssl = true
  # ruleid: ruby-ssl-no-verify
  http.verify_mode = OpenSSL::SSL::VERIFY_NONE
  http
end

def strict(uri)
  http = Net::HTTP.new(uri.host, uri.port)
  http.use_ssl = true
  # ok: ruby-ssl-no-verify
  http.verify_mode = OpenSSL::SSL::VERIFY_PEER
  http
end
