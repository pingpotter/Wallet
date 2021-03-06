package main

import (
	"gopkg.in/mgo.v2"
	"mux"
	"log"
	"net/http"
	"time"
	"encoding/json"
	"crypto/rand"

	"strconv"
	"regexp"
	"io"
	"fmt"
	"strings"
	"io/ioutil"
	"gopkg.in/mgo.v2/bson"
	"net/url"
)

type Acn struct {
	Cizid   int 	`json:"citizen_id" bson:"citizen_id" `
	Wallid   int	`json:"wallet_id,omitempty" bson:"wallet_id"`
	Fname 	string	`json:"full_name" bson:"full_name"`
	Opendate   time.Time	`json:"open_datetime,omitempty" bson:"open_datetime"`
	Balance   float64	`json:"ledger_balance,omitempty" bson:"ledger_balance"`
}

type RqBody struct {
	RqAcn   []Acn    `json:"rqBody"`
}

type rsInqWalletBody struct {
	RsInqWalletAcn   []Acn    `json:"rsBody"`
}

type rsBody struct {
	RsAcn   []RsAcn    `json:"rsBody"`
}

type RsAcn struct {
	Wallid   int    `json:"wallet_id"`
	Opendate   time.Time    `json:"open_datetime"`
}

type ErrorLT struct {
	Errs []Errs	`json:"error"`
}

type Errs struct {
	Ercd   string    `json:"errorCode"`
	Erdes   string    `json:"errorDesc"`
}


func HeaderJSON(w http.ResponseWriter, code int) {

	uuid, _ := newUUID()

	w.Header().Set("x-request-id", uuid)
	w.Header().Set("datetime", time.Now().String())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("x-roundtrip", "")
	w.Header().Set("x-job-id", "")
	w.WriteHeader(code)
}

func main() {
	session, err := mgo.Dial("localhost")
	if err != nil {
		panic(err)
	}
	defer session.Close()

	session.SetMode(mgo.Monotonic, true)
	ensureIndex(session)

	router := mux.NewRouter()

	router.HandleFunc("/v1/accounts", addAcn(session)).Methods("POST")		//Create Wallet Account
	router.HandleFunc("/v1/accounts/search", inqAcnByFname(session)).Methods("GET").Queries()
	router.HandleFunc("/v1/accounts/{walletid}", inqAcnByWallet(session)).Methods("GET")		//inquiry Account by walletID
	router.HandleFunc("/v1/accounts", inqAcnByCizid(session)).Methods("GET").Queries()


	log.Fatal(http.ListenAndServe(":8080", router))

}

func ensureIndex(s *mgo.Session) {
	session := s.Copy()
	defer session.Close()

	c := session.DB("wallet").C("acn")

	index := mgo.Index{
		Key:        []string{"citizen_id"},
		Unique:     true,
		DropDups:   true,
		Background: true,
		Sparse:     true,
	}
	err := c.EnsureIndex(index)
	if err != nil {
		panic(err)
	}
}


func addAcn(s *mgo.Session) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		/*
			test body

			{
				"rqBody": [
				{
				"citizen_id": 1969800106049,
				"full_name": "EOF erer"
				}
			]
			}*/

		session := s.Copy()
		defer session.Close()

		var rqBody RqBody
		var acn Acn
		var errorlt ErrorLT
		var rsbody rsBody
		statCd := http.StatusBadRequest

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {

			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		err = json.Unmarshal(body, &rqBody)
		if err != nil {

			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorlt)
			return
		}


		if len(rqBody.RqAcn) > 1 {

			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:"Incorect body"})
			HeaderJSON(w,http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		for i := 0; i < len(rqBody.RqAcn); i++ {

			//fmt.Println(rqbody.RqAcn[i].Cizid)

			if rqBody.RqAcn[i].Wallid > 0 {

				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"001",Erdes:"Incorect body"})
				statCd = http.StatusBadRequest

				continue

			}

			if rqBody.RqAcn[i].Balance > 0 {

				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"001",Erdes:"Incorect body"})
				statCd = http.StatusBadRequest

				continue

			}

			if rqBody.RqAcn[i].Opendate.Format("20060102") != "00010101" {

				log.Println(rqBody.RqAcn[i].Opendate.Format("20060102"))
				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"001",Erdes:"Incorect body"})
				statCd = http.StatusBadRequest

				continue

			}

			chkZid := chkCIZID(rqBody.RqAcn[i].Cizid)
			strCizid := strconv.Itoa(rqBody.RqAcn[i].Cizid)

			if !chkZid || (len(strconv.Itoa(rqBody.RqAcn[i].Cizid)) > 13) {

				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"001",Erdes:"Incorrect Citizen ID "+strCizid})
				statCd = http.StatusBadRequest

				continue

			}

			var validName = regexp.MustCompile(`^[a-zA-Z.,-]+( [a-zA-Z.,-]+)+$`).MatchString(rqBody.RqAcn[i].Fname)

			if !validName || (len(rqBody.RqAcn[i].Fname) > 50) {

				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"003",Erdes:"Incorrect Name"+rqBody.RqAcn[i].Fname})
				statCd = http.StatusBadRequest

				continue

			}

			c := session.DB("wallet").C("acn")
			cntCizid , err := c.Find("citizen_id").Count()
			cntCizid += 1001

			//Generate wallet id
			runSeq := leftPad2Len(strconv.Itoa(cntCizid), "0", 10)
			chkDigit := strconv.Itoa(creDigit(runSeq))
			wallId := "1"+runSeq+chkDigit

			acn.Cizid = rqBody.RqAcn[i].Cizid
			acn.Fname = strings.ToUpper(rqBody.RqAcn[i].Fname)
			acn.Opendate = time.Now()
			acn.Wallid, _ = strconv.Atoi(wallId)

			err = c.Insert(acn)
			if err != nil {

				if mgo.IsDup(err) {


					errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"002",Erdes:"Duplicate Citizen ID "+strCizid})
					statCd = http.StatusBadRequest
					continue
				}
				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:"Failed insert" })
				statCd = http.StatusInternalServerError
				continue

			}

			rsbody.RsAcn =  append(rsbody.RsAcn,RsAcn{Wallid:acn.Wallid,Opendate:acn.Opendate} )

		}

		if len(errorlt.Errs) > 0 {
			HeaderJSON(w, statCd)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		HeaderJSON(w,http.StatusOK)
		json.NewEncoder(w).Encode(rsbody)

	}
}

func inqAcnByWallet(s *mgo.Session) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		session := s.Copy()
		defer session.Close()

		vars := mux.Vars(r)
		var walletid int
		walletid, _ = strconv.Atoi(vars["walletid"])


		c := session.DB("wallet").C("acn")


		var acn Acn
		var errorlt ErrorLT

		var rsInqWal rsInqWalletBody
		statcd := http.StatusOK
		err := c.Find(bson.M{"wallet_id": walletid}).One(&acn)

		if err != nil {
			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorlt)
			log.Println("Failed find Account : ", err)
			return
		}

		if strconv.Itoa(acn.Wallid) == "" {
			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:"Account not found"})
			HeaderJSON(w,http.StatusNotFound)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		rsInqWal.RsInqWalletAcn =  append(rsInqWal.RsInqWalletAcn,Acn{Cizid:acn.Cizid,Wallid:acn.Wallid,
		Fname:acn.Fname,Opendate:acn.Opendate,Balance:acn.Balance} )

		HeaderJSON(w,statcd)
		json.NewEncoder(w).Encode(rsInqWal)


	}
}

func inqAcnByCizid(s *mgo.Session) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		session := s.Copy()
		defer session.Close()

		m, _ := url.ParseQuery(r.URL.RawQuery)
		citizen, _ := strconv.Atoi(m["citizen_id"][0])

		c := session.DB("wallet").C("acn")

		var acn Acn
		var errorlt ErrorLT

		var rsInqWal rsInqWalletBody
		statcd := http.StatusOK
		err := c.Find(bson.M{"citizen_id": citizen}).One(&acn)

		if err != nil {
			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorlt)
			log.Println("Failed find Account : ", err)
			return
		}

		if strconv.Itoa(acn.Wallid) == "" {
			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:"Account not found"})
			HeaderJSON(w,http.StatusNotFound)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		rsInqWal.RsInqWalletAcn =  append(rsInqWal.RsInqWalletAcn,Acn{Cizid:acn.Cizid,Wallid:acn.Wallid,
			Fname:acn.Fname,Opendate:acn.Opendate,Balance:acn.Balance} )

		HeaderJSON(w,statcd)
		json.NewEncoder(w).Encode(rsInqWal)


	}
}

func inqAcnByFname(s *mgo.Session) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		session := s.Copy()
		defer session.Close()

		m, _ := url.ParseQuery(r.URL.RawQuery)
		Fname := string(m["full_name"][0])
		Fname = ".*"+Fname+".*"

		c := session.DB("wallet").C("acn")

		var acn []Acn
		var errorlt ErrorLT

		var rsInqWal rsInqWalletBody
		statcd := http.StatusOK
		//err := c.Find(bson.M{"full_name": Fname}).All(&acn)
		err := c.Find(bson.M{"full_name": bson.M{"$regex": bson.RegEx{Fname, "i"}}}).All(&acn)

		if err != nil {
			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusInternalServerError)
			json.NewEncoder(w).Encode(errorlt)
			log.Println("Failed find Account : ", err)
			return
		}


		for i := 0; i < len(acn);i++  {
			rsInqWal.RsInqWalletAcn =  append(rsInqWal.RsInqWalletAcn,Acn{Cizid:acn[i].Cizid,Wallid:acn[i].Wallid,
				Fname:acn[i].Fname,Opendate:acn[i].Opendate,Balance:acn[i].Balance} )
		}


		HeaderJSON(w,statcd)
		json.NewEncoder(w).Encode(rsInqWal)


	}
}

func creDigit(runseq string) int{

	sum := 0
	log.Println(len(runseq))
	for i := 0; i < len(runseq) ; i++ {

		intCC, _ := strconv.Atoi(string(runseq[i]))
		sum += intCC *(i+2)
	}

	return sum%10
}

func chkCIZID(cc int) bool{

	strCC := strconv.Itoa(cc)

	if len(strCC) != 13 { return false }

	intCC1, _ := strconv.Atoi(string(strCC[0]))
	if intCC1 == 0 || intCC1 == 9 {return false}

	sum := 0
	for i := 0; i < len(strCC)-1 ; i++ {

		intCC, _ := strconv.Atoi(string(strCC[i]))
		sum += intCC *(13-i)
	}

	intCc12 ,_:= strconv.Atoi(string(strCC[12]))
	if (11 - sum%11)%10 != intCc12 {
		return false
	}

	return true
}

func newUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	// variant bits; see section 4.1.1
	uuid[8] = uuid[8]&^0xc0 | 0x80
	// version 4 (pseudo-random); see section 4.1.3
	uuid[6] = uuid[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func leftPad2Len(s string, padStr string, overallLen int) string {
	var padCountInt int
	padCountInt = 1 + ((overallLen - len(padStr)) / len(padStr))
	var retStr = strings.Repeat(padStr, padCountInt) + s
	return retStr[(len(retStr) - overallLen):]
}
