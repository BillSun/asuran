package proxy

import (
	"github.com/benbearchen/asuran/profile"
	"github.com/benbearchen/asuran/web/proxy/cache"
	"github.com/benbearchen/asuran/web/proxy/life"

	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type htmlUsage struct {
	IP   string
	Host string
}

func (p *Proxy) WriteUsage(w io.Writer) {
	t, err := template.ParseFiles("template/usage.tmpl")

	u := htmlUsage{}
	u.IP = p.serveIP
	u.Host = p.mainHost

	err = t.Execute(w, u)
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

type htmlInitDevice struct {
	ProxyIP  string
	ClientIP string
}

func (p *Proxy) WriteInitDevice(w io.Writer, ip string) {
	t, err := template.ParseFiles("template/i.me.tmpl")

	d := htmlInitDevice{}
	d.ProxyIP = p.serveIP
	d.ClientIP = ip

	err = t.Execute(w, d)
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

type indexData struct {
	Version   string
	ServeIP   string
	ProxyHost string
}

func (p *Proxy) index(w http.ResponseWriter) {
	t, err := template.ParseFiles("template/index.tmpl")
	err = t.Execute(w, indexData{"0.1", p.serveIP, p.mainHost})
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

func (p *Proxy) res(w http.ResponseWriter, r *http.Request, path string) {
	f := filepath.Clean(path)
	h := filepath.Clean("/res/")
	if !strings.HasPrefix(f, h) || len(f) <= len(h) {
		w.WriteHeader(403)
		return
	}

	dir, err := os.Getwd()
	if err != nil {
		w.WriteHeader(404)
		return
	}

	f = filepath.Join(dir, "template", f)
	http.ServeFile(w, r, f)
}

type urlHistoryData struct {
	Even         bool
	ID           string
	Time         string
	Client       string
	Method       string
	ResponseCode string
	RecvBytes    string
}

type urlHistoryListData struct {
	Client    string
	Url       string
	Histories []urlHistoryData
}

func formatUrlHistoryDataList(histories []*cache.UrlHistory, client string) []urlHistoryData {
	d := make([]urlHistoryData, 0, len(histories))
	even := true
	for _, h := range histories {
		even = !even

		responseCode := ""
		if h.ResponseCode >= 0 {
			responseCode = strconv.Itoa(h.ResponseCode)
		}

		recvBytes := "出错"
		if h.Bytes != nil {
			recvBytes = strconv.Itoa(len(h.Bytes))
		}

		d = append(d, urlHistoryData{even, strconv.FormatUint(uint64(h.ID), 10), h.Time.Format("2006-01-02 15:04:05"), client, h.Method, responseCode, recvBytes})
	}

	return d
}

func (p *Proxy) writeUrlHistoryList(w http.ResponseWriter, profileIP, url string, histories []*cache.UrlHistory) {
	t, err := template.ParseFiles("template/history.urllist.tmpl")
	err = t.Execute(w, urlHistoryListData{profileIP, url, formatUrlHistoryDataList(histories, profileIP)})
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

type opData struct {
	Name   string
	Act    string
	Arg    string
	Client string
}

type historyEventData struct {
	Even        bool
	Time        string
	Domain      string
	DomainIP    string
	URL         string
	URLID       string
	URLBody     string
	HttpStatus  string
	EventString string
	OPs         []opData
	Client      string
}

type historyData struct {
	Client string
	Events []historyEventData
}

func formatHistoryEventDataList(events []*life.HistoryEvent, client string, f *life.Life) []historyEventData {
	list := make([]historyEventData, 0, len(events))
	even := true
	for _, e := range events {
		d := historyEventData{}
		d.OPs = make([]opData, 0)
		d.Client = client

		even = !even
		d.Even = even
		d.Time = e.Time.Format("2006-01-02 15:04:05")

		s := strings.Split(e.String, " ")
		if len(s) >= 3 && s[0] == "domain" {
			domain := s[2]
			d.Domain = "域名 " + s[1] + " " + domain
			d.OPs = append(d.OPs, opData{"代理域名", "domain/redirect", domain, client})
			if len(s) >= 4 {
				d.DomainIP = s[3]
			}
		} else if len(s) >= 3 && s[0] == "proxy" {
			url := s[1]
			d.URL = url
			if len(s) >= 4 {
				d.URL += " " + s[3]
			}

			d.URLID = s[2]
			if id, err := strconv.ParseInt(d.URLID, 10, 32); err == nil {
				h := f.LookHistoryByID(uint32(id))
				if h != nil {
					status := h.Method
					if h.ResponseCode >= 0 {
						status += " " + strconv.Itoa(h.ResponseCode)
					} else {
						status += " 出错"
					}

					d.HttpStatus = status
				}
			}

			if strings.HasPrefix(url, "http://") {
				d.URLBody = url[7:]
			} else {
				d.URLBody = url
			}

			d.OPs = append(d.OPs, opData{"缓存", "url/store", d.URLID, client})
		} else {
			d.EventString = e.String
		}

		list = append(list, d)
	}

	return list
}

func (p *Proxy) writeHistory(w http.ResponseWriter, profileIP string, f *life.Life) {
	t, err := template.ParseFiles("template/history.tmpl")
	list := formatHistoryEventDataList(f.HistoryEvents(), profileIP, f)
	err = t.Execute(w, historyData{profileIP, list})
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

type deviceData struct {
	Even     bool
	Name     string
	IP       string
	Owner    string
	InitTime string
}

type devicesListData struct {
	Devices []deviceData
}

func formatDevicesListData(profiles []*profile.Profile, v *life.IPLives) devicesListData {
	devices := make([]deviceData, 0)
	if len(profiles) > 0 {
		even := true
		for _, p := range profiles {
			t := ""
			f := v.OpenExists(p.Ip)
			if f != nil {
				t = f.CreateTime.Format("2006-01-02 15:04:05")
			} else if p.Ip == "localhost" {
				continue
			}

			even = !even

			devices = append(devices, deviceData{even, p.Name, p.Ip, p.Owner, t})
		}
	}

	return devicesListData{devices}
}

func (p *Proxy) devices(w http.ResponseWriter) {
	t, err := template.ParseFiles("template/devices.tmpl")
	profiles := make([]*profile.Profile, 0)
	if p.profileOp != nil {
		profiles = p.profileOp.All()
	}

	err = t.Execute(w, formatDevicesListData(profiles, p.lives))
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

func (p *Proxy) urlEncoded(w http.ResponseWriter) {
	t, err := template.ParseFiles("template/urlencoded.tmpl")
	err = t.Execute(w, nil)
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

type storeData struct {
	Even           bool
	Client         string
	ID             string
	EncodedContent string
}

type storeListData struct {
	Client   string
	Contents []storeData
}

func formatStoreListData(saved []*profile.Store, profileIP string) []storeData {
	s := make([]storeData, 0, len(saved))
	even := true
	for _, v := range saved {
		even = !even
		s = append(s, storeData{even, profileIP, v.ID, url.QueryEscape(string(v.Content))})
	}

	return s
}

func (p *Proxy) writeStores(w http.ResponseWriter, profileIP string, prof *profile.Profile) {
	t, err := template.ParseFiles("template/stores.tmpl")
	list := formatStoreListData(prof.ListStored(), profileIP)
	err = t.Execute(w, storeListData{profileIP, list})
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}

type editStoreData struct {
	Client         string
	ID             string
	EncodedContent string
}

func formatEditStoreData(profileIP string, prof *profile.Profile, id string) editStoreData {
	encodedContent := ""
	if len(id) > 0 {
		c := prof.Restore(id)
		if len(c) > 0 {
			encodedContent = strings.Replace(url.QueryEscape(string(c)), "+", "%20", -1)
		}
	}

	return editStoreData{profileIP, id, encodedContent}
}

func (p *Proxy) writeEditStore(w http.ResponseWriter, profileIP string, prof *profile.Profile, id string) {
	t, err := template.ParseFiles("template/store-edit.tmpl")
	err = t.Execute(w, formatEditStoreData(profileIP, prof, id))
	if err != nil {
		fmt.Fprintln(w, "内部错误：", err)
	}
}
