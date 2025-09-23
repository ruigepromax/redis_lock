if (redis.call('hexists', KEYS[1], ARGV[2]) == 0) then
				return 0
				end
				redis.call('del', KEYS[1])
				redis.call('PUBLISH', KEYS[2], ARGV[1])
				return 1