# grab go-fdo-client at revision (TODO(runcom): stick to a revision)
git clone https://github.com/fido-device-onboard/go-fdo-client
cd go-fdo-client
go build -o fdo_client ./cmd/fdo_client
cd ../
rm -rf mfg.db rv.db own.db
# start manufacturing server
go-fdo-server serve 127.0.0.1:8038 --db ./mfg.db --db-pass 1234Cia0@! --debug &
timeout 300 bash -c 'while [[ "$(curl -s -o /dev/null -w ''%{http_code}'' 127.0.0.1:8038/health)" != "200" ]]; do sleep 5; done' || false
# start owner server
go-fdo-server serve 127.0.0.1:8041 --db ./rv.db --db-pass 1234Cia0@! --debug &
timeout 300 bash -c 'while [[ "$(curl -s -o /dev/null -w ''%{http_code}'' 127.0.0.1:8041/health)" != "200" ]]; do sleep 5; done' || false
# start rendezvous server
go-fdo-server serve 127.0.0.1:8043 --db ./own.db --db-pass 1234Cia0@! --debug &
timeout 300 bash -c 'while [[ "$(curl -s -o /dev/null -w ''%{http_code}'' 127.0.0.1:8043/health)" != "200" ]]; do sleep 5; done' || false
# creating new rv info data works
# but when onboarding, the rv throws a not found... TODO(runcom): investigate
# https://github.com/fido-device-onboard/go-fdo-server?tab=readme-ov-file#create-new-rv-info-data
# Create New Owner Redirect Data
curl --location --request POST 'http://127.0.0.1:8043/api/v1/owner/redirect' \
--header 'Content-Type: text/plain' \
--data-raw '[["127.0.0.1","127.0.0.1",8043,3]]'
# run device initialization
# TODO(runcom): tpm initialization missing
rm -rf cred.bin
./go-fdo-client/fdo_client -di-device-info=gotest -di http://127.0.0.1:8038 -debug
# grab guid
GUID=$(./go-fdo-client/fdo_client -print | grep GUID | awk '{print $2}')
# fetch and post voucher
curl --location --request GET 'http://127.0.0.1:8038/api/v1/vouchers?guid='${GUID} -o ownervoucher
curl -v -X POST 'http://127.0.0.1:8041/api/v1/owner/vouchers' -d @ownervoucher
curl -v -X POST 'http://127.0.0.1:8043/api/v1/owner/vouchers' -d @ownervoucher
# run onboarding
./go-fdo-client/fdo_client -debug | grep 'FIDO Device Onboard Complete'