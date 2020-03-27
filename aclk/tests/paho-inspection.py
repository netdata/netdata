import ssl
import paho.mqtt.client as mqtt
import json
import time
import sys

def on_connect(mqttc, obj, flags, rc):
    if rc==0:
        print("Successful connection", flush=True)
    else :
        print(f"Connection error rc={rc}", flush=True)
    mqttc.subscribe("/agent/#",0)

def on_disconnect(mqttc, obj, flags, rc):
    print("disconnected rc: "+str(rc), flush=True)

def on_message(mqttc, obj, msg):
    print(f"{msg.topic} {len(msg.payload)}-bytes qos={msg.qos}", flush=True)
    try:
        print(f"Trying decode of {msg.payload[:60]}",flush=True)
        api_msg = json.loads(msg.payload)
    except Exception as e:
        print(e,flush=True)
        return
    ts = api_msg["timestamp"]
    mtype = api_msg["type"]
    print(f"Message {mtype} time={ts} size {len(api_msg)}", flush=True)
    now = time.time()
    print(f"Current {now} -> Delay {now-ts}", flush=True)
    if mtype=="disconnect":
        print(f"Message dump: {api_msg}", flush=True)

def on_publish(mqttc, obj, mid):
    print("mid: "+str(mid), flush=True)

def on_subscribe(mqttc, obj, mid, granted_qos):
    print("Subscribed: "+str(mid)+" "+str(granted_qos), flush=True)

def on_log(mqttc, obj, level, string):
    print(string)

print(f"Starting paho-inspection on {sys.argv[1]}", flush=True)
mqttc = mqtt.Client(transport='websockets',client_id="paho")
#mqttc.tls_set(certfile="server.crt", keyfile="server.key", cert_reqs=ssl.CERT_REQUIRED, tls_version=ssl.PROTOCOL_TLS, ciphers=None)
#mqttc.tls_set(ca_certs="server.crt", cert_reqs=ssl.CERT_REQUIRED, tls_version=ssl.PROTOCOL_TLS, ciphers=None)
mqttc.tls_set(cert_reqs=ssl.CERT_NONE, tls_version=ssl.PROTOCOL_TLS, ciphers=None)
mqttc.tls_insecure_set(True)
mqttc.on_message = on_message
mqttc.on_connect = on_connect
mqttc.on_disconnect = on_disconnect
mqttc.on_publish = on_publish
mqttc.on_subscribe = on_subscribe
mqttc.username_pw_set("paho","paho")
mqttc.connect(sys.argv[1], 8443, 60)

#mqttc.publish("/agent/mine","Test1")
#mqttc.subscribe("$SYS/#", 0)
print("Connected succesfully, monitoring /agent/#", flush=True)
mqttc.loop_forever()
