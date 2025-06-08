#!/bin/bash
ffmpeg -hide_banner -encoders | grep -E '^ V' | grep -F '(codec' | cut -c 8- | sort
