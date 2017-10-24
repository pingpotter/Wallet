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
)


type Acn struct {
	Cizid   int 	`bson:"wallet_id"`
	Wallid   int	`bson:"citizen_id"`
	Fname 	string	`bson:"full_name"`
	Opendate   time.Time	`bson:"open_datetime"`
	Balance   float64	`bson:"ledger_balance"`
}

type rqBody struct {
	RqAcn   []RqAcn    `json:"rqBody"`
}

type RqAcn struct {
	Cizid   int    `json:"citizen_id"`
	Fname 	string  `json:"full_name"`
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

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("datetime", time.Now().String())
	w.Header().Set("x-roundtrip", "")
	w.Header().Set("x-job-id", "")
	w.Header().Set("x-request-id", uuid)
	w.WriteHeader(code)
}

func main() {
	session, err := mgo.Dial("localhost")
	if err != nil {
		panic(err)
	}
	defer session.Close()

	session.SetMode(mgo.Monotonic, true)
	ensureIndex2(session)

	router := mux.NewRouter()

	router.HandleFunc("/v1/accounts", addAcn(session)).Methods("POST")		//Create Wallet Account

	log.Fatal(http.ListenAndServe(":8080", router))

}

func ensureIndex2(s *mgo.Session) {
	session := s.Copy()
	defer session.Close()

	c := session.DB("wallet").C("acn")

	index := mgo.Index{
		Key:        []string{"cizid"},
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
		{
			"rqBody": [
			{
			"cizid": 1969800106049,
			"fname": "EOF erer"
			}
		]
		}*/

		session := s.Copy()
		defer session.Close()

		var rqbody rqBody
		var acn Acn
		var errorlt ErrorLT

		body, err := ioutil.ReadAll(r.Body)
		if err != nil {

			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		err = json.Unmarshal(body, &rqbody)
		if err != nil {

			errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:string(err.Error())})
			HeaderJSON(w,http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorlt)
			return
		}


		var rsbody rsBody
		statcd := http.StatusCreated

		zerr := false

		for i := 0; i < len(rqbody.RqAcn); i++ {

			fmt.Println(rqbody.RqAcn[i].Cizid)

			chkz := chkCIZID(rqbody.RqAcn[i].Cizid)
			zcid := strconv.Itoa(rqbody.RqAcn[i].Cizid)

			if !chkz {

				zerr = true
				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"001",Erdes:"Incorrect Citizen ID "+zcid})
				statcd = http.StatusBadRequest

				continue

			}

			var validName = regexp.MustCompile(`^[a-zA-Z.,-]+( [a-zA-Z.,-]+)+$`).MatchString(rqbody.RqAcn[i].Fname)

			if !validName {

				zerr = true
				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"003",Erdes:"Incorrect Name"+rqbody.RqAcn[i].Fname})
				statcd = http.StatusBadRequest

				continue

			}


			c := session.DB("wallet").C("acn")
			cntCizid , err := c.Find("cizid").Count()
			//log.Println(cntCizid)
			cntCizid += 1001
			//Generate wallet id
			runseq := leftPad2Len(strconv.Itoa(cntCizid), "0", 10)
			chkdigit := strconv.Itoa(creDigit(runseq))
			wallid := "1"+runseq+chkdigit

			acn.Cizid = rqbody.RqAcn[i].Cizid
			acn.Fname = rqbody.RqAcn[i].Fname
			acn.Opendate = time.Now()
			acn.Wallid, _ = strconv.Atoi(wallid)

			err = c.Insert(acn)
			if err != nil {

				zerr = true

				if mgo.IsDup(err) {


					errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"002",Erdes:"Duplicate Citizen ID "+zcid})
					statcd = http.StatusBadRequest
					continue
				}
				errorlt.Errs = append(errorlt.Errs,Errs{Ercd:"9999",Erdes:"Failed insert" })
				statcd = http.StatusInternalServerError
				continue

				log.Println("Failed insert book: ", err)
			}


			rsbody.RsAcn =  append(rsbody.RsAcn,RsAcn{Wallid:acn.Wallid,Opendate:acn.Opendate} )

		}

		if zerr {
			HeaderJSON(w, statcd)
			json.NewEncoder(w).Encode(errorlt)
			return
		}

		HeaderJSON(w,statcd)

		json.NewEncoder(w).Encode(rsbody)

	}
}

func creDigit(runseq string) int{

	sum := 0
	log.Println(len(runseq))
	for i := 0; i < len(runseq) ; i++ {

		log.Println(i)
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