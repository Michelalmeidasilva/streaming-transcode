import bdrate

def test_bdrate_self_is_zero():
    rates = [500, 1000, 2000, 4000]
    vmaf  = [70.0, 82.0, 91.0, 97.0]
    bd = bdrate.bd_rate(rates, vmaf, rates, vmaf)
    assert abs(bd) < 1e-6

def test_bdrate_cheaper_curve_is_negative():
    r_ref  = [1000, 2000, 4000, 8000]
    v_ref  = [70.0, 82.0, 91.0, 97.0]
    r_test = [800, 1600, 3200, 6400]   # 20% less bitrate at every point
    bd = bdrate.bd_rate(r_ref, v_ref, r_test, v_ref)
    assert abs(bd - (-20.0)) < 0.5  # exact: (0.8 - 1) * 100 = -20.0%
