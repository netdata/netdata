//go:build ignore
// +build ignore

package ibm_mq

import (
	"context"
	"fmt"

	"github.com/ibm-messaging/mq-golang/ibmmq"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	mx := make(map[string]int64)

	qMgr, err := c.connect()
	if err != nil {
		return nil, err
	}
	defer qMgr.Disc()

	if err := c.collectQueueMetrics(qMgr, mx); err != nil {
		c.Error(err)
	}

	if err := c.collectChannelMetrics(qMgr, mx); err != nil {
		c.Error(err)
	}

	if err := c.collectQueueManagerMetrics(qMgr, mx); err != nil {
		c.Error(err)
	}

	return mx, nil
}

func (c *Collector) connect() (*ibmmq.MQQueueManager, error) {
	cno := ibmmq.NewMQCNO()
	cd := ibmmq.NewMQCD()

	cd.ChannelName = c.conf.Channel
	cd.ConnectionName = fmt.Sprintf("%s(%d)", c.conf.Host, c.conf.Port)
	cno.ClientConn = cd

	if c.conf.User != "" {
		csp := ibmmq.NewMQCSP()
		csp.AuthenticationType = ibmmq.MQCSP_AUTH_USER_ID_AND_PWD
		csp.UserId = c.conf.User
		csp.Password = c.conf.Password
		cno.SecurityParms = csp
	}

	qMgr, err := ibmmq.Connx(c.conf.QueueManager, cno)
	if err != nil {
		return nil, err
	}

	return &qMgr, nil
}

func (c *Collector) collectQueueMetrics(qMgr *ibmmq.MQQueueManager, mx map[string]int64) error {
	openOptions := ibmmq.MQOO_INQUIRE | ibmmq.MQOO_FAIL_IF_QUIESCING

	qod := ibmmq.NewMQOD()
	qod.ObjectType = ibmmq.MQOT_Q
	qod.ObjectName = "*"

	q, err := qMgr.Open(qod, openOptions)
	if err != nil {
		return err
	}
	defer q.Close(0)

	selectors := []int32{ibmmq.MQIA_CURRENT_Q_DEPTH, ibmmq.MQIA_MAX_Q_DEPTH, ibmmq.MQIA_MSG_DEQ_COUNT, ibmmq.MQIA_MSG_ENQ_COUNT, ibmmq.MQIA_OPEN_INPUT_COUNT, ibmmq.MQIA_OPEN_OUTPUT_COUNT}
	intAttrs := make([]int32, len(selectors))

	for {
		mqErr, err := q.Inq(selectors, intAttrs, nil)
		if err != nil {
			return err
		}

		if mqErr.MQCC == ibmmq.MQCC_FAILED && mqErr.MQRC == ibmmq.MQRC_NO_MORE_MSGS {
			break
		}

		if mqErr.MQCC != ibmmq.MQCC_OK {
			return fmt.Errorf("q.Inq() failed with MQCC %d and MQRC %d", mqErr.MQCC, mqErr.MQRC)
		}

		qName, err := q.InqString(ibmmq.MQCA_Q_NAME)
		if err != nil {
			return err
		}

		dimPrefix := "ibm_mq.queue_" + qName
		mx[dimPrefix+"_depth_current"] = int64(intAttrs[0])
		mx[dimPrefix+"_depth_max"] = int64(intAttrs[1])
		mx[dimPrefix+"_msg_deq_count"] = int64(intAttrs[2])
		mx[dimPrefix+"_msg_enq_count"] = int64(intAttrs[3])
		mx[dimPrefix+"_open_input_count"] = int64(intAttrs[4])
		mx[dimPrefix+"_open_output_count"] = int64(intAttrs[5])

		// Calculate depth percentage (0-100)
		if intAttrs[1] > 0 { // Avoid division by zero
			mx[dimPrefix+"_depth_percent"] = (int64(intAttrs[0]) * 100) / int64(intAttrs[1])
		} else {
			mx[dimPrefix+"_depth_percent"] = 0
		}
	}

	return nil
}

func (c *Collector) collectChannelMetrics(qMgr *ibmmq.MQQueueManager, mx map[string]int64) error {
	adminQueue, err := openQueue(qMgr, "SYSTEM.ADMIN.COMMAND.QUEUE", ibmmq.MQOO_OUTPUT)
	if err != nil {
		return err
	}
	defer adminQueue.Close(0)

	replyQueue, err := openQueue(qMgr, "SYSTEM.DEFAULT.MODEL.QUEUE", ibmmq.MQOO_INPUT_EXCLUSIVE)
	if err != nil {
		return err
	}
	defer replyQueue.Close(0)

	if err := sendPCFChannelInquiry(qMgr, adminQueue, replyQueue.Name, "*"); err != nil {
		return err
	}

	return getPCFChannelResponse(qMgr, replyQueue, mx)
}

func sendPCFChannelInquiry(qMgr *ibmmq.MQQueueManager, qObject ibmmq.MQObject, replyToQueue string, channelToInquire string) error {
	putmqmd := ibmmq.NewMQMD()
	pmo := ibmmq.NewMQPMO()

	putmqmd.Format = ibmmq.MQFMT_ADMIN
	putmqmd.ReplyToQ = replyToQueue
	putmqmd.MsgType = ibmmq.MQMT_REQUEST
	pmo.Options = ibmmq.MQPMO_NO_SYNCPOINT | ibmmq.MQPMO_NEW_MSG_ID | ibmmq.MQPMO_NEW_CORREL_ID

	cfh := ibmmq.NewMQCFH()
	cfh.Command = ibmmq.MQCMD_INQUIRE_CHANNEL_STATUS

	pcfparm := new(ibmmq.PCFParameter)
	pcfparm.Type = ibmmq.MQCFT_STRING
	pcfparm.Parameter = ibmmq.MQCACH_CHANNEL_NAME
	pcfparm.String = []string{channelToInquire}
	cfh.ParameterCount++

	pcfparm2 := new(ibmmq.PCFParameter)
	pcfparm2.Type = ibmmq.MQCFT_INTEGER
	pcfparm2.Parameter = ibmmq.MQIACH_CHANNEL_INSTANCE_TYPE
	pcfparm2.Int64Value = []int64{int64(ibmmq.MQOT_CURRENT_CHANNEL)}
	cfh.ParameterCount++

	buf := append(cfh.Bytes(), pcfparm.Bytes()...)
	buf = append(buf, pcfparm2.Bytes()...)

	return qObject.Put(putmqmd, pmo, buf)
}

func getPCFChannelResponse(qMgr *ibmmq.MQQueueManager, qObject ibmmq.MQObject, mx map[string]int64) error {
	getmqmd := ibmmq.NewMQMD()
	gmo := ibmmq.NewMQGMO()
	gmo.Options = ibmmq.MQGMO_NO_SYNCPOINT | ibmmq.MQGMO_WAIT | ibmmq.MQGMO_CONVERT
	gmo.WaitInterval = 3000 // 3 seconds

	buffer := make([]byte, 32768)
	datalen, err := qObject.Get(getmqmd, gmo, buffer)
	if err != nil {
		mqret := err.(*ibmmq.MQReturn)
		if mqret.MQRC == ibmmq.MQRC_NO_MSG_AVAILABLE {
			return fmt.Errorf("no response message from command server")
		}
		return err
	}

	responses, err := ibmmq.ParsePCF(buffer[0:datalen])
	if err != nil {
		return err
	}

	for _, pcfResponse := range responses {
		if pcfResponse.Command == ibmmq.MQCMD_INQUIRE_CHANNEL_STATUS {
			channelName := pcfResponse.GetString(ibmmq.MQCACH_CHANNEL_NAME)
			dimPrefix := "ibm_mq.channel_" + channelName[0]

			mx[dimPrefix+"_batches"] = pcfResponse.GetInt64(ibmmq.MQIACH_BATCHES)[0]
			mx[dimPrefix+"_buffers_rcvd"] = pcfResponse.GetInt64(ibmmq.MQIACH_BUFFERS_RCVD)[0]
			mx[dimPrefix+"_buffers_sent"] = pcfResponse.GetInt64(ibmmq.MQIACH_BUFFERS_SENT)[0]
			mx[dimPrefix+"_bytes_rcvd"] = pcfResponse.GetInt64(ibmmq.MQIACH_BYTES_RCVD)[0]
			mx[dimPrefix+"_bytes_sent"] = pcfResponse.GetInt64(ibmmq.MQIACH_BYTES_SENT)[0]
			mx[dimPrefix+"_current_msgs"] = pcfResponse.GetInt64(ibmmq.MQIACH_CURRENT_MSGS)[0]
			mx[dimPrefix+"_msgs"] = pcfResponse.GetInt64(ibmmq.MQIACH_MSGS)[0]
		}
	}

	return nil
}

func openQueue(qMgr *ibmmq.MQQueueManager, queueName string, options int32) (ibmmq.MQObject, error) {
	mqod := ibmmq.NewMQOD()
	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = queueName

	return qMgr.Open(mqod, options)
}

func (c *Collector) collectQueueManagerMetrics(qMgr *ibmmq.MQQueueManager, mx map[string]int64) error {
	qod := ibmmq.NewMQOD()
	qod.ObjectType = ibmmq.MQOT_Q_MGR
	qod.ObjectName = c.conf.QueueManager

	openOptions := ibmmq.MQOO_INQUIRE | ibmmq.MQOO_FAIL_IF_QUIESCING

	q, err := qMgr.Open(qod, openOptions)
	if err != nil {
		return err
	}
	defer q.Close(0)

	selectors := []int32{ibmmq.MQIA_DIST_LISTS, ibmmq.MQIACH_MAX_MSG_LENGTH}
	intAttrs := make([]int32, len(selectors))

	_, err = q.Inq(selectors, intAttrs, nil)
	if err != nil {
		return err
	}

	mx["ibm_mq.queue_manager_dist_lists"] = int64(intAttrs[0])
	mx["ibm_mq.queue_manager_max_msg_list"] = int64(intAttrs[1])

	return nil
}
