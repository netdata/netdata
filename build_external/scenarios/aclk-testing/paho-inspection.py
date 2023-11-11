import ssl
import paho.mqtt.client as mqtt

def on_connect(mqttc, obj, flags, rc):
    print(f"connected rc: {str(rc)}", flush=True)
    mqttc.subscribe("/agent/#",0)
def on_disconnect(mqttc, obj, flags, rc):
    print(f"disconnected rc: {str(rc)}", flush=True)
def on_message(mqttc, obj, msg):
    print(f"{msg.topic} {str(msg.qos)} {str(msg.payload)}", flush=True)
def on_publish(mqttc, obj, mid):
    print(f"mid: {str(mid)}", flush=True)
def on_subscribe(mqttc, obj, mid, granted_qos):
    print(f"Subscribed: {str(mid)} {str(granted_qos)}", flush=True)
def on_log(mqttc, obj, level, string):
    print(string)
print("Starting paho-inspection", flush=True)
mqttc = mqtt.Client(transport='websockets')
#mqttc.tls_set(certfile="server.crt", keyfile="server.key", cert_reqs=ssl.CERT_REQUIRED, tls_version=ssl.PROTOCOL_TLS, ciphers=None)
#mqttc.tls_set(ca_certs="server.crt", cert_reqs=ssl.CERT_REQUIRED, tls_version=ssl.PROTOCOL_TLS, ciphers=None)
mqttc.tls_set(cert_reqs=ssl.CERT_NONE, tls_version=ssl.PROTOCOL_TLS, ciphers=None)
mqttc.tls_insecure_set(True)
mqttc.on_message = on_message
mqttc.on_connect = on_connect
mqttc.on_disconnect = on_disconnect
mqttc.on_publish = on_publish
mqttc.on_subscribe = on_subscribe
mqttc.connect("vernemq", 9002, 60)

#mqttc.publish("/agent/mine","Test1")
#mqttc.subscribe("$SYS/#", 0)
print("Connected successfully, monitoring /agent/#", flush=True)
mqttc.loop_forever()
