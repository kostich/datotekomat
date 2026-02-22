#!/bin/bash
export OUTPUT=8;

# build the fat12 tool
echo "===test&build==="
rm ./datotekomat &>/dev/null;
go clean -testcache
go test -cover ./...
export TEST_STATUS=$(echo $?)
export CGO_ENABLED=0
go build || exit 1
mkdir -p ~/.local/bin
export PATH="$PATH:~/.local/bin"
mv -vf ./datotekomat ~/.local/bin/датотекомат

# test it
rm -fv ./*.img ./*.слк ./datotekomat &>/dev/null

# make a new filesystem
echo "===     датотекомат       ===" && \
датотекомат -в фмт -ус 8 \
  -бпс 16 -уст 4 -етк "ЈСДАТ-1" проба.слк && \
  xxd -a -g 1 -c $OUTPUT проба.слк 2>/dev/null;
датотекомат -в осб проба.слк

echo "=== величина ==="
touch проба.слк &>/dev/null
ls -la проба.слк;

# copy a file into the new filesystem
printf '9 chars.\n' > file9.txt
датотекомат -в кпу file9.txt / проба.слк && \
  xxd -a -g 1 -c $OUTPUT проба.слк 2>/dev/null
датотекомат -в осб проба.слк

echo "=== величина ==="
touch проба.слк &>/dev/null
ls -la проба.слк;

if [[ $TEST_STATUS -eq 0 ]]; then
  echo -e "\033[0;32mТестови ПРОЛАЗЕ.\033[0m"
else
  echo -e "\033[0;31mТестови НЕ ПРОЛАЗЕ.\033[0m"
fi

# cleanup
rm -f file9.txt проба.слк


