package router

import "testing"

const desc = `<?xml version="1.0"?><root><device><friendlyName>Home Router</friendlyName><modelName>RX-1</modelName>
<deviceList><device><deviceList><device><serviceList><service>
<serviceType>urn:schemas-upnp-org:service:WANCommonInterfaceConfig:1</serviceType><controlURL>/ctl/CmnIfCfg</controlURL>
</service></serviceList></device></deviceList></device></deviceList></device></root>`

func TestParseDescription(t *testing.T) {
	g, err := parseDescription([]byte(desc), "http://192.168.1.1:5000/rootDesc.xml")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Home Router" || g.controlURL != "http://192.168.1.1:5000/ctl/CmnIfCfg" {
		t.Fatalf("%+v", g)
	}
	if _, err := parseDescription([]byte(`<root><device/></root>`), "http://x/"); err != ErrNoIGD {
		t.Fatal(err)
	}
}

func TestSoapField(t *testing.T) {
	body := `<s:Envelope><s:Body><u:GetTotalBytesReceivedResponse><NewTotalBytesReceived>123456</NewTotalBytesReceived></u:GetTotalBytesReceivedResponse></s:Body></s:Envelope>`
	v, err := soapField([]byte(body), "NewTotalBytesReceived")
	if err != nil || v != "123456" {
		t.Fatal(v, err)
	}
}

func TestDelta(t *testing.T) {
	if Delta(100, 150) != 50 || Delta(1<<32-10, 5) != 15 || Delta(1<<40, 5) != 0 {
		t.Fatal("delta")
	}
}
