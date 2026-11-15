package service

import "sort"

// 两个平台都要做的汇总：谁像被爆破、失败次数最多的来源 IP。

// bruteForceFrom keeps users with at least 5 failures, biggest first.
func bruteForceFrom(failByUser map[string]map[string]bool, failByIP map[string]int) []BruteForce {
	var out []BruteForce
	for user, ips := range failByUser {
		total := 0
		for ip := range ips {
			total += failByIP[ip]
		}
		if total < 5 {
			continue
		}
		ipList := make([]string, 0, len(ips))
		for ip := range ips {
			ipList = append(ipList, ip)
		}
		sort.Strings(ipList)
		out = append(out, BruteForce{User: user, Count: total, IPs: ipList})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// topIPs returns the N busiest source addresses.
func topIPs(failByIP map[string]int, n int) []IPCount {
	type kv struct {
		ip    string
		count int
	}
	arr := make([]kv, 0, len(failByIP))
	for ip, c := range failByIP {
		arr = append(arr, kv{ip, c})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].count == arr[j].count {
			return arr[i].ip < arr[j].ip
		}
		return arr[i].count > arr[j].count
	})
	out := make([]IPCount, 0, n)
	for i, e := range arr {
		if i >= n {
			break
		}
		out = append(out, IPCount{IP: e.ip, Count: e.count})
	}
	return out
}
