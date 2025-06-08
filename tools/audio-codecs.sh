#!/bin/bash
ffmpeg -hide_banner -encoders | grep -E '^ A' | grep -F '(codec' | cut -c 8- | sort
