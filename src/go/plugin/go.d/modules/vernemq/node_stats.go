// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

//type (
//	vernemqNode struct {
//		name string
//
//		ns nodeStats
//		m4 *mqtt4Stats
//		m5 *mqtt5Stats
//	}
//	nodeStats struct {
//		sockets struct {
//			open         int64
//			closed       int64
//			error        int64
//			closeTimeout int64
//		}
//		queue struct {
//			processes int64
//			setup     int64
//			teardown  int64
//			message   struct {
//				in        int64
//				out       int64
//				drop      int64
//				expired   int64
//				unhandled int64
//			}
//			initFromStorage int64
//		}
//		router struct {
//			matchesLocal  int64
//			matchesRemote int64
//			memory        int64
//			subscriptions int64
//		}
//		erlangVm struct {
//			system struct {
//				utilization     int64
//				processCount    int64
//				reductions      int64
//				contextSwitches int64
//				io              struct {
//					in  int64
//					out int64
//				}
//				runQueue           int64
//				gcCount            int64
//				wordsReclaimedByGC int64
//			}
//			vm struct {
//				memoryProcesses int64
//				memorySystem    int64
//			}
//		}
//		bandwidth struct {
//			receivedBytes int64
//			sentBytes     int64
//		}
//		retain struct {
//			memory   int64
//			messages int64
//		}
//		cluster struct {
//			bytes struct {
//				received int64
//				sent     int64
//				dropped  int64
//			}
//			netsplit struct {
//				detected int64
//				resolved int64
//			}
//		}
//		wallclock int64
//	}
//
//	mqtt5Stats struct {
//		mqttCommonStats
//	}
//	mqtt4Stats struct {
//		mqttCommonStats
//	}
//	mqttCommonStats struct {
//		connect struct {
//			received int64
//			sent     int64
//		}
//		subscribe struct {
//			received  int64
//			sent      int64
//			error     int64
//			authError int64
//		}
//		unsubscribe struct {
//			received int64
//			sent     int64
//			error    int64
//		}
//		publish struct {
//			received  int64
//			sent      int64
//			error     int64
//			authError int64
//		}
//		puback struct {
//			received int64
//			sent     int64
//			invalid  int64
//		}
//		pubrec struct {
//			received int64
//			sent     int64
//			//invalidError int64
//		}
//		pubrel struct {
//			received int64
//			sent     int64
//		}
//	}
//)
